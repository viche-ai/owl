package tools

import (
	"fmt"
	"strings"

	"github.com/viche-ai/owl/internal/viche"
)

// VicheTools provides discover and send tools backed by a live Viche channel
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
						"description": "Capability to search for (e.g. 'coding', 'code-review'). Use '*' for all.",
					},
				},
				"required": []string{"capability"},
			},
		},
		{
			Name:        "viche_send",
			Description: "Send a message to another AI agent on the Viche network.",
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

	if to == "" || body == "" {
		return "Error: 'to' and 'body' are required"
	}

	_, err := vt.Channel.Push("send_message", map[string]interface{}{
		"to":   to,
		"body": body,
		"type": "task",
	})
	if err != nil {
		return fmt.Sprintf("Failed to send message: %v", err)
	}

	return fmt.Sprintf("Message sent to %s.", to)
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
