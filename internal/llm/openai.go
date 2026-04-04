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

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type OpenAIProvider struct {
	apiKey  string
	baseURL string
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &OpenAIProvider{apiKey: apiKey, baseURL: baseURL}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error) {
	return p.ChatStreamWithTools(ctx, model, messages, nil)
}

func (p *OpenAIProvider) ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
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
				tcs = append(tcs, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
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

	// Debug: write request body to log
	_ = os.MkdirAll("/tmp/owl-debug", 0755)
	debugFile := fmt.Sprintf("/tmp/owl-debug/openai-%d.json", time.Now().UnixMilli())
	_ = os.WriteFile(debugFile, bodyBytes, 0644)

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}

	// Some providers (Gemini) may return JSON errors even on 200 with stream=true
	// Check if the response is actually SSE
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") && !strings.Contains(ct, "application/x-ndjson") {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		// Try to parse as error
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

	// Debug: log raw SSE to file
	home, _ := os.UserHomeDir()
	_ = os.MkdirAll(home+"/.owl/debug", 0755)
	sseLogPath := fmt.Sprintf("%s/.owl/debug/sse-%d.log", home, time.Now().UnixMilli())
	sseLog, _ := os.Create(sseLogPath)

	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		if sseLog != nil {
			defer func() { _ = sseLog.Close() }()
		}

		// Track tool call assembly across chunks
		toolCalls := make(map[int]*ToolCallEvent)

		scanner := bufio.NewScanner(resp.Body)
		// Increase scanner buffer for large responses
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if sseLog != nil {
				_, _ = fmt.Fprintf(sseLog, "%s\n", line)
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamEvent{Done: true}
				return
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// Check for error responses
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

				if fr, ok := choice["finish_reason"].(string); ok && (fr == "tool_calls" || fr == "stop") && len(toolCalls) > 0 {
					for _, tc := range toolCalls {
						ch <- StreamEvent{ToolCall: tc}
					}
					toolCalls = make(map[int]*ToolCallEvent)
				}
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

		// Emit any tool calls that weren't flushed (e.g. stream ended without finish_reason)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
