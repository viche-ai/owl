package llm

import "context"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	// For assistant messages that contain tool_use blocks (Anthropic format)
	ToolCalls []ToolCallRef `json:"tool_calls,omitempty"`
}

// ToolCallRef stores a reference to a tool call made by the assistant
type ToolCallRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type StreamEvent struct {
	Delta    string
	Done     bool
	Usage    *Usage
	Error    error
	ToolCall *ToolCallEvent
}

type ToolCallEvent struct {
	ID        string
	Name      string
	Arguments string
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ContextKey string

const (
	SessionIDKey ContextKey = "session_id"
)

type Provider interface {
	ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error)
	ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
	Name() string
}
