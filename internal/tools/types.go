package tools

import (
	"encoding/json"
	"fmt"
)

// ToolDefinition describes a tool the LLM can call
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents the LLM requesting a tool execution
type ToolCall struct {
	Name string
	Args map[string]interface{}
}

// ParseToolCallFromJSON parses tool_use blocks from Anthropic or function_call from OpenAI
func ParseToolCallFromJSON(name string, argsJSON string) (ToolCall, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolCall{}, fmt.Errorf("failed to parse tool args: %w", err)
	}
	return ToolCall{Name: name, Args: args}, nil
}
