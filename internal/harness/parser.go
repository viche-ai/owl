package harness

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OutputEvent represents a parsed event from harness stdout.
type OutputEvent struct {
	Type    string                 // "text", "tool_call", "usage", "error", "status"
	Content string                 // the displayable text
	Meta    map[string]interface{} // structured data (e.g. token counts for "usage")
}

// OutputParser processes lines of harness stdout into structured events.
type OutputParser interface {
	ProcessLine(line string) []OutputEvent
}

// NewParser returns a parser for the given output format.
// Supported formats: "text" (default), "ndjson", "claude-stream-json".
func NewParser(format string) OutputParser {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "ndjson", "json":
		return &ndjsonParser{}
	case "claude-stream-json":
		return &claudeStreamParser{}
	default:
		return &textParser{}
	}
}

// textParser is a pass-through that wraps each line as a text event.
type textParser struct{}

func (p *textParser) ProcessLine(line string) []OutputEvent {
	return []OutputEvent{{Type: "text", Content: line}}
}

// ndjsonParser parses each line as a JSON object.
// Recognized fields:
//
//	{"type": "text",      "content": "..."}
//	{"type": "usage",     "input_tokens": N, "output_tokens": N}
//	{"type": "tool_call", "name": "...", "args": "..."}
//	{"type": "error",     "message": "..."}
//	{"type": "status",    "state": "..."}
//
// Lines that fail JSON parsing are emitted as text events.
type ndjsonParser struct{}

func (p *ndjsonParser) ProcessLine(line string) []OutputEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		// Not valid JSON — treat as plain text
		return []OutputEvent{{Type: "text", Content: line}}
	}

	eventType, _ := raw["type"].(string)
	if eventType == "" {
		eventType = "text"
	}

	switch eventType {
	case "usage":
		return []OutputEvent{{
			Type: "usage",
			Meta: raw,
		}}

	case "tool_call":
		name, _ := raw["name"].(string)
		return []OutputEvent{{
			Type:    "tool_call",
			Content: name,
			Meta:    raw,
		}}

	case "error":
		msg, _ := raw["message"].(string)
		if msg == "" {
			msg, _ = raw["content"].(string)
		}
		return []OutputEvent{{
			Type:    "error",
			Content: msg,
			Meta:    raw,
		}}

	case "status":
		state, _ := raw["state"].(string)
		return []OutputEvent{{
			Type:    "status",
			Content: state,
			Meta:    raw,
		}}

	default:
		// "text" or any unrecognized type
		content, _ := raw["content"].(string)
		if content == "" {
			content = line
		}
		return []OutputEvent{{
			Type:    "text",
			Content: content,
			Meta:    raw,
		}}
	}
}

// ExtractTokenUsage extracts input and output token counts from a "usage" event's Meta.
// Returns (0, 0) if the fields are missing or not numeric.
func ExtractTokenUsage(ev OutputEvent) (input int, output int) {
	if ev.Type != "usage" || ev.Meta == nil {
		return 0, 0
	}
	input = jsonInt(ev.Meta, "input_tokens")
	output = jsonInt(ev.Meta, "output_tokens")
	return
}

// claudeStreamParser handles Claude Code's --output-format stream-json.
//
// Event types emitted by Claude Code:
//
//	{"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
//	{"type":"tool_use","tool":{"name":"Read","input":{...}}}
//	{"type":"tool_result","tool":{"name":"Read"},"content":"..."}
//	{"type":"result","result":"...","cost_usd":0.01,"input_tokens":N,"output_tokens":N,"duration_ms":N}
type claudeStreamParser struct{}

func (p *claudeStreamParser) ProcessLine(line string) []OutputEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return []OutputEvent{{Type: "text", Content: line}}
	}

	eventType, _ := raw["type"].(string)

	switch eventType {
	case "assistant":
		// Extract text from message.content array
		text := extractAssistantText(raw)
		if text == "" {
			return nil
		}
		return []OutputEvent{{Type: "text", Content: text, Meta: raw}}

	case "tool_use":
		name := extractToolName(raw)
		return []OutputEvent{{Type: "tool_call", Content: name, Meta: raw}}

	case "tool_result":
		name := extractToolName(raw)
		content, _ := raw["content"].(string)
		preview := content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return []OutputEvent{{
			Type:    "text",
			Content: fmt.Sprintf("[%s] %s", name, preview),
			Meta:    raw,
		}}

	case "result":
		var events []OutputEvent

		// Extract final result text
		if result, ok := raw["result"].(string); ok && result != "" {
			events = append(events, OutputEvent{Type: "text", Content: result, Meta: raw})
		}

		// Extract usage
		inputTokens := jsonInt(raw, "input_tokens")
		outputTokens := jsonInt(raw, "output_tokens")
		if inputTokens > 0 || outputTokens > 0 {
			events = append(events, OutputEvent{
				Type: "usage",
				Meta: map[string]interface{}{
					"input_tokens":  raw["input_tokens"],
					"output_tokens": raw["output_tokens"],
					"cost_usd":      raw["cost_usd"],
					"duration_ms":   raw["duration_ms"],
				},
			})
		}

		return events

	default:
		// Pass through unknown event types as text
		content, _ := raw["content"].(string)
		if content == "" {
			content = line
		}
		return []OutputEvent{{Type: "text", Content: content, Meta: raw}}
	}
}

func extractAssistantText(raw map[string]interface{}) string {
	msg, ok := raw["message"].(map[string]interface{})
	if !ok {
		return ""
	}
	contentArr, ok := msg["content"].([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range contentArr {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if blockType, _ := block["type"].(string); blockType == "text" {
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func extractToolName(raw map[string]interface{}) string {
	tool, ok := raw["tool"].(map[string]interface{})
	if !ok {
		return "unknown"
	}
	name, _ := tool["name"].(string)
	if name == "" {
		return "unknown"
	}
	return name
}

func jsonInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
