package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viche-ai/owl/internal/viche"
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

// VicheTools provides discover/send/reply tools backed by a live Viche channel
type VicheTools struct {
	Channel *viche.Channel
	AgentID string
}

func (vt *VicheTools) Definitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "viche_discover",
			Description: "Discover other AI agents on the Viche network by capability. Pass '*' to list all agents.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"capability": map[string]interface{}{
						"type":        "string",
						"description": "Capability to search for (e.g. 'coding', 'owl-agent'). Use '*' for all.",
					},
				},
				"required": []string{"capability"},
			},
		},
		{
			Name:        "viche_send",
			Description: "Send a message to another AI agent on the Viche network. Use this to delegate tasks, ask new questions, or initiate a new topic with an agent.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Target agent ID (UUID)",
					},
					"body": map[string]interface{}{
						"type":        "string",
						"description": "Message content",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Message type: 'task', 'result', or 'ping'",
					},
				},
				"required": []string{"to", "body"},
			},
		},
		{
			Name:        "viche_reply",
			Description: "Reply to an agent that sent you a task to acknowledge receipt or provide the final answer. DO NOT use this to ask follow-up questions; use viche_send instead.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Agent ID to reply to (from the inbound message's 'from' field)",
					},
					"body": map[string]interface{}{
						"type":        "string",
						"description": "Your result or response",
					},
				},
				"required": []string{"to", "body"},
			},
		},
	}
}

func (vt *VicheTools) Execute(call ToolCall) string {
	switch call.Name {
	case "viche_discover":
		return vt.discover(call.Args)
	case "viche_send":
		return vt.send(call.Args)
	case "viche_reply":
		return vt.reply(call.Args)
	default:
		return fmt.Sprintf("Unknown tool: %s", call.Name)
	}
}

func (vt *VicheTools) discover(args map[string]interface{}) string {
	capability, _ := args["capability"].(string)
	if capability == "" {
		return "Error: capability is required"
	}

	resp, err := vt.Channel.Push("discover", map[string]interface{}{
		"capability": capability,
	})
	if err != nil {
		return fmt.Sprintf("Discovery failed: %v", err)
	}

	agents, ok := resp["agents"].([]interface{})
	if !ok || len(agents) == 0 {
		return "No agents found matching that capability."
	}

	var lines []string
	for _, a := range agents {
		if agent, ok := a.(map[string]interface{}); ok {
			id, _ := agent["id"].(string)
			name, _ := agent["name"].(string)
			caps := "unknown"
			if c, ok := agent["capabilities"].([]interface{}); ok {
				var cs []string
				for _, cap := range c {
					if s, ok := cap.(string); ok {
						cs = append(cs, s)
					}
				}
				caps = strings.Join(cs, ", ")
			}
			nameStr := ""
			if name != "" {
				nameStr = fmt.Sprintf(" (%s)", name)
			}
			lines = append(lines, fmt.Sprintf("• %s%s — capabilities: %s", id, nameStr, caps))
		}
	}

	return fmt.Sprintf("Found %d agent(s):\n%s", len(agents), strings.Join(lines, "\n"))
}

func (vt *VicheTools) send(args map[string]interface{}) string {
	to, _ := args["to"].(string)
	body, _ := args["body"].(string)
	msgType, _ := args["type"].(string)
	if msgType == "" {
		msgType = "task"
	}

	if to == "" || body == "" {
		return "Error: 'to' and 'body' are required"
	}

	_, err := vt.Channel.Push("send_message", map[string]interface{}{
		"to":   to,
		"body": body,
		"type": msgType,
	})
	if err != nil {
		return fmt.Sprintf("Failed to send message: %v", err)
	}

	return fmt.Sprintf("Message sent to %s (type: %s).", to, msgType)
}

func (vt *VicheTools) reply(args map[string]interface{}) string {
	args["type"] = "result"
	return vt.send(args)
}

// ToAnthropicTools converts tool definitions to the Anthropic API tool format
func (vt *VicheTools) ToAnthropicTools() []map[string]interface{} {
	var tools []map[string]interface{}
	for _, def := range vt.Definitions() {
		tools = append(tools, map[string]interface{}{
			"name":         def.Name,
			"description":  def.Description,
			"input_schema": def.Parameters,
		})
	}
	return tools
}

// ToOpenAITools converts tool definitions to the OpenAI API function calling format
func (vt *VicheTools) ToOpenAITools() []map[string]interface{} {
	var tools []map[string]interface{}
	for _, def := range vt.Definitions() {
		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        def.Name,
				"description": def.Description,
				"parameters":  def.Parameters,
			},
		})
	}
	return tools
}

// ParseToolCallsFromJSON parses tool_use blocks from Anthropic or function_call from OpenAI
func ParseToolCallFromJSON(name string, argsJSON string) (ToolCall, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolCall{}, fmt.Errorf("failed to parse tool args: %w", err)
	}
	return ToolCall{Name: name, Args: args}, nil
}
