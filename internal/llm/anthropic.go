package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultAnthropicBaseURL = "https://api.anthropic.com"
const anthropicAPIVersion = "2023-06-01"

type AnthropicProvider struct {
	apiKey  string
	baseURL string
}

func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &AnthropicProvider{apiKey: apiKey, baseURL: baseURL}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error) {
	return p.ChatStreamWithTools(ctx, model, messages, nil)
}

func (p *AnthropicProvider) ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	var systemPrompt string
	var chatMessages []map[string]interface{}

	for _, m := range messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
		} else if m.Role == RoleTool {
			// Anthropic: tool_result in a user message
			chatMessages = append(chatMessages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})
		} else if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			// Assistant message with tool_use blocks
			var content []map[string]interface{}
			if m.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				json.Unmarshal([]byte(tc.Arguments), &input)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			chatMessages = append(chatMessages, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
		} else {
			chatMessages = append(chatMessages, map[string]interface{}{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}

	body := map[string]interface{}{
		"model":      model,
		"messages":   chatMessages,
		"max_tokens": 8192,
		"stream":     true,
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	if len(tools) > 0 {
		var anthropicTools []map[string]interface{}
		for _, t := range tools {
			anthropicTools = append(anthropicTools, map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			})
		}
		body["tools"] = anthropicTools
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	// Debug: write request body to log
	os.MkdirAll("/tmp/owl-debug", 0755)
	debugFile := fmt.Sprintf("/tmp/owl-debug/anthropic-%d.json", time.Now().UnixMilli())
	os.WriteFile(debugFile, bodyBytes, 0644)

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		var currentToolID string
		var currentToolName string
		var currentToolArgs strings.Builder

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "content_block_start":
				if cb, ok := event["content_block"].(map[string]interface{}); ok {
					if cbType, _ := cb["type"].(string); cbType == "tool_use" {
						currentToolID, _ = cb["id"].(string)
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
					ch <- StreamEvent{
						ToolCall: &ToolCallEvent{
							ID:        currentToolID,
							Name:      currentToolName,
							Arguments: currentToolArgs.String(),
						},
					}
					currentToolName = ""
					currentToolID = ""
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
				ch <- StreamEvent{Done: true}
				return

			case "error":
				if errData, ok := event["error"].(map[string]interface{}); ok {
					errMsg, _ := errData["message"].(string)
					errType, _ := errData["type"].(string)
					ch <- StreamEvent{Error: fmt.Errorf("%s: %s", errType, errMsg)}
				} else {
					ch <- StreamEvent{Error: fmt.Errorf("unknown streaming error")}
				}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Error: err}
		}
	}()

	return ch, nil
}
