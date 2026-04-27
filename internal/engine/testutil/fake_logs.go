package testutil

import "sync"

type FakeLogEvent struct {
	Level      string
	Message    string
	ToolName   string
	ToolArgs   string
	ToolResult string
	ModelID    string
	TokensIn   int
	TokensOut  int
}

type FakeLogWriter struct {
	mu     sync.Mutex
	Events []FakeLogEvent
	Closed bool
}

func (f *FakeLogWriter) Log(level, message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, FakeLogEvent{
		Level:   level,
		Message: message,
	})
}

func (f *FakeLogWriter) LogTool(toolName, args, result string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, FakeLogEvent{
		Level:      "tool",
		Message:    "[Tool: " + toolName + "] " + args,
		ToolName:   toolName,
		ToolArgs:   args,
		ToolResult: result,
	})
}

func (f *FakeLogWriter) LogUsage(tokensIn, tokensOut int, modelID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, FakeLogEvent{
		Level:     "info",
		ModelID:   modelID,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
	})
}

func (f *FakeLogWriter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Closed = true
	return nil
}
