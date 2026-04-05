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

const minimaxBaseURL = "https://api.minimax.io/v1"

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
	var oaiMessages []map[string]interface{}
	for _, m := range messages {
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			var tcs []map[string]interface{}
			for _, tc := range m.ToolCalls {
				tcObj := map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
				tcs = append(tcs, tcObj)
			}
			msg["tool_calls"] = tcs
		}
		oaiMessages = append(oaiMessages, msg)
	}

	body := map[string]interface{}{
		"model":    model,
		"messages": oaiMessages,
		"stream":   true,
	}

	if len(tools) > 0 {
		var oaiTools []map[string]interface{}
		for _, t := range tools {
			oaiTools = append(oaiTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = oaiTools
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	_ = os.MkdirAll("/tmp/owl-debug", 0755)
	debugFile := fmt.Sprintf("/tmp/owl-debug/minimax-%d.json", time.Now().UnixMilli())
	_ = os.WriteFile(debugFile, bodyBytes, 0644)

	req, err := http.NewRequestWithContext(ctx, "POST", minimaxBaseURL+"/text/chatcompletion_v2", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		toolCalls := make(map[int]*ToolCallEvent)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" || data == "DONE" {
				ch <- StreamEvent{Done: true}
				return
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if errObj, ok := chunk["error"].(map[string]interface{}); ok {
				errMsg, _ := errObj["message"].(string)
				ch <- StreamEvent{Error: fmt.Errorf("API error: %s", errMsg)}
				return
			}

			choices, _ := chunk["choices"].([]interface{})
			if len(choices) > 0 {
				choice, _ := choices[0].(map[string]interface{})
				delta, _ := choice["delta"].(map[string]interface{})

				if content, ok := delta["content"].(string); ok && content != "" {
					ch <- StreamEvent{Delta: content}
				}

				if tcs, ok := delta["tool_calls"].([]interface{}); ok {
					for i, tc := range tcs {
						tcMap, _ := tc.(map[string]interface{})
						idx := i

						if idxVal, ok := tcMap["index"].(float64); ok {
							idx = int(idxVal)
						}

						if _, exists := toolCalls[idx]; !exists {
							toolCalls[idx] = &ToolCallEvent{}
						}
						if id, ok := tcMap["id"].(string); ok {
							toolCalls[idx].ID = id
						}
						if fn, ok := tcMap["function"].(map[string]interface{}); ok {
							if name, ok := fn["name"].(string); ok {
								toolCalls[idx].Name = name
							}
							if args, ok := fn["arguments"].(string); ok {
								toolCalls[idx].Arguments += args
							}
						}
					}
				}

				if finishReason, ok := choice["finish_reason"].(string); ok && finishReason == "tool_calls" && len(toolCalls) > 0 {
					for _, tc := range toolCalls {
						ch <- StreamEvent{ToolCall: tc}
					}
					toolCalls = make(map[int]*ToolCallEvent)
				}

				if usage, ok := chunk["usage"].(map[string]interface{}); ok {
					pt, _ := usage["prompt_tokens"].(float64)
					ct, _ := usage["completion_tokens"].(float64)
					tt, _ := usage["total_tokens"].(float64)
					ch <- StreamEvent{Usage: &Usage{
						PromptTokens:     int(pt),
						CompletionTokens: int(ct),
						TotalTokens:      int(tt),
					}}
				}
			}
		}

		if len(toolCalls) > 0 {
			for _, tc := range toolCalls {
				ch <- StreamEvent{ToolCall: tc}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Error: err}
		}
	}()

	return ch, nil
}
