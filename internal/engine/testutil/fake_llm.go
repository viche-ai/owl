package testutil

import (
	"context"
	"sync"

	"github.com/viche-ai/owl/internal/llm"
)

type FakeLLM struct {
	mu        sync.Mutex
	Script    [][]llm.StreamEvent
	Calls     []FakeLLMCall
	NextError error
}

type FakeLLMCall struct {
	Model    string
	Messages []llm.Message
	Tools    []llm.ToolDef
}

func (f *FakeLLM) ChatStream(ctx context.Context, model string, messages []llm.Message) (<-chan llm.StreamEvent, error) {
	return f.chat(model, messages, nil)
}

func (f *FakeLLM) ChatStreamWithTools(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDef) (<-chan llm.StreamEvent, error) {
	return f.chat(model, messages, tools)
}

func (f *FakeLLM) Name() string {
	return "fake"
}

func (f *FakeLLM) chat(model string, messages []llm.Message, tools []llm.ToolDef) (<-chan llm.StreamEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, FakeLLMCall{
		Model:    model,
		Messages: append([]llm.Message(nil), messages...),
		Tools:    append([]llm.ToolDef(nil), tools...),
	})

	if f.NextError != nil {
		err := f.NextError
		f.NextError = nil
		return nil, err
	}

	var script []llm.StreamEvent
	if len(f.Script) > 0 {
		script = append([]llm.StreamEvent(nil), f.Script[0]...)
		f.Script = f.Script[1:]
	}

	ch := make(chan llm.StreamEvent, len(script)+1)
	for _, event := range script {
		ch <- event
	}
	close(ch)
	return ch, nil
}
