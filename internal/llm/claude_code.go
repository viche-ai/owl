package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ClaudeCodeProvider struct{}

func NewClaudeCodeProvider() *ClaudeCodeProvider {
	return &ClaudeCodeProvider{}
}

func (p *ClaudeCodeProvider) Name() string { return "cli/claude-code" }

func (p *ClaudeCodeProvider) ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error) {
	return p.ChatStreamWithTools(ctx, model, messages, nil)
}

func (p *ClaudeCodeProvider) ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// We only take the last message because Claude Code CLI manages its own history
	lastMsg := messages[len(messages)-1]
	prompt := lastMsg.Content

	sessionID, ok := ctx.Value("session_id").(string)
	if !ok || sessionID == "" {
		sessionID = "00000000-0000-0000-0000-000000000001"
	} else {
		// Claude Code requires a strict UUID format. If it's just "1", we pad it to a UUID format.
		if len(sessionID) < 32 {
			sessionID = fmt.Sprintf("00000000-0000-0000-0000-%012s", sessionID)
			// Replace spaces with zeros just in case
			sessionID = strings.ReplaceAll(sessionID, " ", "0")
		}
	}

	// Note: tools are not passed via CLI arguments directly here.
	// In the future, owl will dynamically generate an MCP config and pass --mcp-config owl-<uuid>.json

	cmdArgs := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--session-id", sessionID,
	}

	cmd := exec.CommandContext(ctx, "claude", cmdArgs...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude CLI: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		var currentToolName string
		var currentToolArgs strings.Builder

		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var wrapper map[string]interface{}
			if err := json.Unmarshal([]byte(line), &wrapper); err != nil {
				continue
			}

			// We are looking for native Anthropic stream events wrapped by the CLI
			if wrapper["type"] != "stream_event" {
				continue
			}

			event, ok := wrapper["event"].(map[string]interface{})
			if !ok {
				continue
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "content_block_start":
				if cb, ok := event["content_block"].(map[string]interface{}); ok {
					if cbType, _ := cb["type"].(string); cbType == "tool_use" {
						currentToolName, _ = cb["name"].(string)
						currentToolArgs.Reset()
					}
				}

			case "content_block_delta":
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					deltaType, _ := delta["type"].(string)
					if deltaType == "text_delta" {
						if text, ok := delta["text"].(string); ok && text != "" {
							ch <- StreamEvent{Delta: text}
						}
					} else if deltaType == "input_json_delta" {
						if partial, ok := delta["partial_json"].(string); ok {
							currentToolArgs.WriteString(partial)
						}
					}
				}

			case "content_block_stop":
				if currentToolName != "" {
					// We just emit the tool use as plain text so the user can see what Claude is doing natively.
					// We DO NOT emit a ToolCallEvent, because Claude Code CLI manages its own tool execution
					// (and uses MCP for Owl tools), so we don't want the Owl engine to try executing it again.
					toolOutput := fmt.Sprintf("\n> [Native Tool: %s] %s\n", currentToolName, currentToolArgs.String())
					ch <- StreamEvent{Delta: toolOutput}

					currentToolName = ""
					currentToolArgs.Reset()
				}

			case "message_delta":
				if usage, ok := event["usage"].(map[string]interface{}); ok {
					input, _ := usage["input_tokens"].(float64)
					output, _ := usage["output_tokens"].(float64)
					ch <- StreamEvent{
						Usage: &Usage{
							PromptTokens:     int(input),
							CompletionTokens: int(output),
							TotalTokens:      int(input + output),
						},
					}
				}

			case "message_stop":
				// We DO NOT emit Done here. Claude Code CLI might execute a tool natively
				// and then stream more events (another message). We wait for the process to exit.
				continue

			case "error":
				if errData, ok := event["error"].(map[string]interface{}); ok {
					errMsg, _ := errData["message"].(string)
					errType, _ := errData["type"].(string)
					ch <- StreamEvent{Error: fmt.Errorf("%s: %s", errType, errMsg)}
				} else {
					ch <- StreamEvent{Error: fmt.Errorf("unknown streaming error")}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Error: err}
		}

		// Wait for process to exit cleanly
		_ = cmd.Wait()

		// If we haven't sent a Done, send one now to ensure the UI updates
		ch <- StreamEvent{Done: true}
	}()

	return ch, nil
}
