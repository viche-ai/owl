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

const minimaxBaseURL = "https://api.minimax.io/anthropic/v1"

type MinimaxProvider struct {
	apiKey string
}

func NewMinimaxProvider(apiKey string) *MinimaxProvider {
	return &MinimaxProvider{apiKey: apiKey}
}

func (p *MinimaxProvider) Name() string { return "minimax" }

func (p *MinimaxProvider) ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error) {
	return p.ChatStreamWithTools(ctx, model, messages, nil)
}

func (p *MinimaxProvider) ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	var systemPrompt string
	var chatMessages []map[string]interface{}

	for _, m := range messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
		} else if m.Role == RoleTool {
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
			var content []map[string]interface{}
			if m.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
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
		"extra_body": map[string]interface{}{
			"reasoning_split": true,
		},
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

	_ = os.MkdirAll("/tmp/owl-debug", 0755)
	debugFile := fmt.Sprintf("/tmp/owl-debug/minimax-%d.json", time.Now().UnixMilli())
	_ = os.WriteFile(debugFile, bodyBytes, 0644)

	req, err := http.NewRequestWithContext(ctx, "POST", minimaxBaseURL+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") && !strings.Contains(ct, "application/x-ndjson") {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(b, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("unexpected response (Content-Type: %s): %s", ct, string(b[:min(len(b), 200)]))
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		var currentToolID string
		var currentToolName string
		var currentToolArgs strings.Builder
		var reasoningBuffer strings.Builder
		var contentBuffer strings.Builder

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamEvent{Done: true}
				return
			}

			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			if errObj, ok := event["error"].(map[string]interface{}); ok {
				errMsg, _ := errObj["message"].(string)
				ch <- StreamEvent{Error: fmt.Errorf("API error: %s", errMsg)}
				return
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "message_start":
				if usage, ok := event["usage"].(map[string]interface{}); ok {
					pt, _ := usage["input_tokens"].(float64)
					ct, _ := usage["output_tokens"].(float64)
					ch <- StreamEvent{Usage: &Usage{
						PromptTokens:     int(pt),
						CompletionTokens: int(ct),
					}}
				}

			case "content_block_start":
				contentBlock, _ := event["content_block"].(map[string]interface{})
				blockType, _ := contentBlock["type"].(string)
				if blockType == "tool_use" {
					currentToolID, _ = contentBlock["id"].(string)
					currentToolName, _ = contentBlock["name"].(string)
					currentToolArgs.Reset()
				}

			case "content_block_delta":
				delta, _ := event["delta"].(map[string]interface{})
				switch delta["type"] {
				case "text_delta":
					if text, ok := delta["text"].(string); ok {
						contentBuffer.WriteString(text)
						ch <- StreamEvent{Delta: text}
					}
				case "thinking_delta":
					if text, ok := delta["thinking"].(string); ok {
						reasoningBuffer.WriteString(text)
						ch <- StreamEvent{Reasoning: text}
					}
				case "input_reasoning_delta":
					if text, ok := delta["input_reasoning"].(string); ok {
						reasoningBuffer.WriteString(text)
						ch <- StreamEvent{Reasoning: text}
					}
				case "tool_use_delta":
					if text, ok := delta["input"].(string); ok {
						currentToolArgs.WriteString(text)
					}
				}

			case "content_block_stop":
				if currentToolID != "" && currentToolName != "" {
					ch <- StreamEvent{ToolCall: &ToolCallEvent{
						ID:        currentToolID,
						Name:      currentToolName,
						Arguments: currentToolArgs.String(),
					}}
					currentToolID = ""
					currentToolName = ""
				}

			case "message_delta":
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					if stopReason, ok := delta["stop_reason"].(string); ok {
						if stopReason == "tool_use" {
							if currentToolID != "" && currentToolName != "" {
								ch <- StreamEvent{ToolCall: &ToolCallEvent{
									ID:        currentToolID,
									Name:      currentToolName,
									Arguments: currentToolArgs.String(),
								}}
							}
						}
					}
				}
				if usage, ok := event["usage"].(map[string]interface{}); ok {
					pt, _ := usage["input_tokens"].(float64)
					ct, _ := usage["output_tokens"].(float64)
					tt, _ := usage["total_tokens"].(float64)
					ch <- StreamEvent{Usage: &Usage{
						PromptTokens:     int(pt),
						CompletionTokens: int(ct),
						TotalTokens:      int(tt),
					}}
				}
				ch <- StreamEvent{Done: true}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Error: err}
		}
	}()

	return ch, nil
}
