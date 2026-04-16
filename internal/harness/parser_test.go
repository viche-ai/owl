package harness

import (
	"strings"
	"testing"
)

func TestTextParser_Passthrough(t *testing.T) {
	p := NewParser("text")
	events := p.ProcessLine("hello world")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "text" {
		t.Fatalf("expected type 'text', got %q", events[0].Type)
	}
	if events[0].Content != "hello world" {
		t.Fatalf("expected content 'hello world', got %q", events[0].Content)
	}
}

func TestTextParser_EmptyLine(t *testing.T) {
	p := NewParser("text")
	events := p.ProcessLine("")
	if len(events) != 1 || events[0].Content != "" {
		t.Fatalf("text parser should pass through empty lines, got %v", events)
	}
}

func TestTextParser_DefaultFormat(t *testing.T) {
	// Empty/unknown format should default to text
	p := NewParser("")
	events := p.ProcessLine("test")
	if len(events) != 1 || events[0].Type != "text" {
		t.Fatal("empty format should default to text parser")
	}

	p2 := NewParser("unknown-format")
	events2 := p2.ProcessLine("test")
	if len(events2) != 1 || events2[0].Type != "text" {
		t.Fatal("unknown format should default to text parser")
	}
}

func TestNDJSONParser_TextEvent(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "text", "content": "hello"}`)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "text" || events[0].Content != "hello" {
		t.Fatalf("expected text event with 'hello', got %+v", events[0])
	}
}

func TestNDJSONParser_UsageEvent(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "usage", "input_tokens": 1500, "output_tokens": 300}`)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "usage" {
		t.Fatalf("expected type 'usage', got %q", events[0].Type)
	}

	input, output := ExtractTokenUsage(events[0])
	if input != 1500 {
		t.Fatalf("expected input_tokens=1500, got %d", input)
	}
	if output != 300 {
		t.Fatalf("expected output_tokens=300, got %d", output)
	}
}

func TestNDJSONParser_ToolCallEvent(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "tool_call", "name": "file_read", "args": "{\"path\":\"/tmp/x\"}"}`)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "tool_call" || events[0].Content != "file_read" {
		t.Fatalf("expected tool_call 'file_read', got %+v", events[0])
	}
}

func TestNDJSONParser_ErrorEvent(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "error", "message": "something broke"}`)
	if len(events) != 1 || events[0].Type != "error" || events[0].Content != "something broke" {
		t.Fatalf("expected error event, got %+v", events)
	}
}

func TestNDJSONParser_StatusEvent(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "status", "state": "thinking"}`)
	if len(events) != 1 || events[0].Type != "status" || events[0].Content != "thinking" {
		t.Fatalf("expected status event, got %+v", events)
	}
}

func TestNDJSONParser_InvalidJSON(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine("this is not json at all")
	if len(events) != 1 || events[0].Type != "text" {
		t.Fatal("invalid JSON should fall back to text event")
	}
	if events[0].Content != "this is not json at all" {
		t.Fatalf("content should be the raw line, got %q", events[0].Content)
	}
}

func TestNDJSONParser_NoTypeField(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"content": "hello", "extra": 42}`)
	if len(events) != 1 || events[0].Type != "text" || events[0].Content != "hello" {
		t.Fatalf("missing type should default to text, got %+v", events)
	}
}

func TestNDJSONParser_EmptyLine(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine("")
	if len(events) != 0 {
		t.Fatalf("empty line should produce no events, got %d", len(events))
	}
}

func TestNDJSONParser_WhitespaceLine(t *testing.T) {
	p := NewParser("ndjson")
	events := p.ProcessLine("   ")
	if len(events) != 0 {
		t.Fatalf("whitespace line should produce no events, got %d", len(events))
	}
}

func TestNDJSONParser_ErrorFallbackToContent(t *testing.T) {
	// Error event with "content" instead of "message"
	p := NewParser("ndjson")
	events := p.ProcessLine(`{"type": "error", "content": "fallback msg"}`)
	if len(events) != 1 || events[0].Content != "fallback msg" {
		t.Fatalf("error should fall back to 'content' field, got %+v", events)
	}
}

func TestExtractTokenUsage_NonUsageEvent(t *testing.T) {
	ev := OutputEvent{Type: "text", Content: "hello"}
	input, output := ExtractTokenUsage(ev)
	if input != 0 || output != 0 {
		t.Fatalf("non-usage event should return 0,0, got %d,%d", input, output)
	}
}

func TestExtractTokenUsage_MissingFields(t *testing.T) {
	ev := OutputEvent{Type: "usage", Meta: map[string]interface{}{}}
	input, output := ExtractTokenUsage(ev)
	if input != 0 || output != 0 {
		t.Fatalf("missing fields should return 0,0, got %d,%d", input, output)
	}
}

func TestNewParser_JSONAlias(t *testing.T) {
	// "json" should also work as alias for ndjson
	p := NewParser("json")
	events := p.ProcessLine(`{"type": "text", "content": "hi"}`)
	if len(events) != 1 || events[0].Content != "hi" {
		t.Fatal("'json' format should use ndjson parser")
	}
}

// ── Claude stream-json parser tests ──

func TestClaudeStreamParser_AssistantText(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the bug."}]}}`
	events := p.ProcessLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "text" || events[0].Content != "I'll fix the bug." {
		t.Fatalf("expected text 'I'll fix the bug.', got %+v", events[0])
	}
}

func TestClaudeStreamParser_AssistantMultipleBlocks(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"First."},{"type":"text","text":" Second."}]}}`
	events := p.ProcessLine(line)
	if len(events) != 1 || events[0].Content != "First. Second." {
		t.Fatalf("expected concatenated text, got %+v", events)
	}
}

func TestClaudeStreamParser_AssistantEmptyContent(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"assistant","message":{"role":"assistant","content":[]}}`
	events := p.ProcessLine(line)
	if len(events) != 0 {
		t.Fatalf("empty assistant content should produce no events, got %d", len(events))
	}
}

func TestClaudeStreamParser_ToolUse(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"tool_use","tool":{"name":"Read","input":{"file_path":"/tmp/x"}}}`
	events := p.ProcessLine(line)
	if len(events) != 1 || events[0].Type != "tool_call" || events[0].Content != "Read" {
		t.Fatalf("expected tool_call 'Read', got %+v", events)
	}
}

func TestClaudeStreamParser_ToolResult(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"tool_result","tool":{"name":"Read"},"content":"file contents here"}`
	events := p.ProcessLine(line)
	if len(events) != 1 || events[0].Type != "text" {
		t.Fatalf("expected text event for tool_result, got %+v", events)
	}
	if !strings.Contains(events[0].Content, "Read") || !strings.Contains(events[0].Content, "file contents here") {
		t.Fatalf("tool_result should show name and content, got %q", events[0].Content)
	}
}

func TestClaudeStreamParser_ToolResultTruncated(t *testing.T) {
	p := NewParser("claude-stream-json")
	longContent := strings.Repeat("x", 500)
	line := `{"type":"tool_result","tool":{"name":"Read"},"content":"` + longContent + `"}`
	events := p.ProcessLine(line)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	// Content should be truncated: "[Read] " prefix + 200 chars + "..."
	if len(events[0].Content) > 250 {
		t.Fatalf("tool_result content should be truncated, got length %d", len(events[0].Content))
	}
}

func TestClaudeStreamParser_Result(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"result","result":"All done.","cost_usd":0.0089,"input_tokens":2048,"output_tokens":512,"duration_ms":3200}`
	events := p.ProcessLine(line)

	// Should produce a text event + a usage event
	if len(events) != 2 {
		t.Fatalf("expected 2 events (text + usage), got %d: %+v", len(events), events)
	}

	if events[0].Type != "text" || events[0].Content != "All done." {
		t.Fatalf("first event should be text, got %+v", events[0])
	}

	if events[1].Type != "usage" {
		t.Fatalf("second event should be usage, got %+v", events[1])
	}
	input, output := ExtractTokenUsage(events[1])
	if input != 2048 || output != 512 {
		t.Fatalf("expected 2048/512 tokens, got %d/%d", input, output)
	}
}

func TestClaudeStreamParser_ResultNoTokens(t *testing.T) {
	p := NewParser("claude-stream-json")
	line := `{"type":"result","result":"Done."}`
	events := p.ProcessLine(line)
	// Only text, no usage event
	if len(events) != 1 || events[0].Type != "text" {
		t.Fatalf("expected 1 text event, got %+v", events)
	}
}

func TestClaudeStreamParser_InvalidJSON(t *testing.T) {
	p := NewParser("claude-stream-json")
	events := p.ProcessLine("not json at all")
	if len(events) != 1 || events[0].Type != "text" || events[0].Content != "not json at all" {
		t.Fatalf("invalid JSON should fall back to text, got %+v", events)
	}
}

func TestClaudeStreamParser_EmptyLine(t *testing.T) {
	p := NewParser("claude-stream-json")
	events := p.ProcessLine("")
	if len(events) != 0 {
		t.Fatalf("empty line should produce no events, got %d", len(events))
	}
}

func TestClaudeStreamParser_UnknownType(t *testing.T) {
	p := NewParser("claude-stream-json")
	events := p.ProcessLine(`{"type":"system","content":"hello"}`)
	if len(events) != 1 || events[0].Type != "text" || events[0].Content != "hello" {
		t.Fatalf("unknown type should fall back to text, got %+v", events)
	}
}
