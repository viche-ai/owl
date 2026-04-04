package ipc

import (
	"fmt"
	"sync"
)

type AgentState struct {
	ID        string
	Name      string
	Role      string
	Ctx       string
	State     string
	Logs      string
	VicheID   string
	Registry  string
	ModelID   string
	Thinking  bool
	Effort    string
	Verbosity string
}

type Service struct {
	Mu     sync.Mutex
	Agents []*AgentState

	// Channel-based message routing: agentIndex -> channel of inbound messages
	InboxChans map[int]chan InboundMessage
}

type InboundMessage struct {
	From    string // "user" or a viche agent ID
	Content string
}

func NewService() *Service {
	return &Service{
		Agents:     []*AgentState{},
		InboxChans: make(map[int]chan InboundMessage),
	}
}

type HatchArgs struct {
	Description string
	Registry    string
	ModelID     string
	Template    string
	Thinking    bool
	Effort      string
	Name        string
	Ambient     bool
}

type HatchReply struct {
	Success bool
	Message string
}

var RunEngineHook func(state *AgentState, mu func(func()), args *HatchArgs, inbox chan InboundMessage)

func (s *Service) Hatch(args *HatchArgs, reply *HatchReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	fmt.Println("=> [Daemon] Received hatch request:", args.Description)

	idx := len(s.Agents)

	name := args.Name
	if name == "" {
		name = args.Description
	}

	newAgent := &AgentState{
		ID:        fmt.Sprintf("%d", idx+1),
		Name:      name,
		Role:      "auto",
		Ctx:       "0 / 128k",
		State:     "hatching",
		Logs:      "",
		ModelID:   args.ModelID,
		Thinking:  args.Thinking,
		Effort:    args.Effort,
		Verbosity: "verbose", // default verbosity
	}

	s.Agents = append(s.Agents, newAgent)

	// Create an inbox channel for this agent
	inbox := make(chan InboundMessage, 64)
	s.InboxChans[idx] = inbox

	if RunEngineHook != nil {
		go RunEngineHook(newAgent, func(f func()) {
			s.Mu.Lock()
			defer s.Mu.Unlock()
			f()
		}, args, inbox)
	}

	reply.Success = true
	reply.Message = "Egg hatched for: " + args.Description
	return nil
}

// SendMessage delivers a user message to an agent's inbox
type SendMessageArgs struct {
	AgentIndex int
	Content    string
}

type SendMessageReply struct {
	Success bool
}

func (s *Service) SendMessage(args *SendMessageArgs, reply *SendMessageReply) error {
	s.Mu.Lock()
	ch, ok := s.InboxChans[args.AgentIndex]
	s.Mu.Unlock()

	if !ok {
		return fmt.Errorf("no inbox for agent index %d", args.AgentIndex)
	}

	ch <- InboundMessage{From: "user", Content: args.Content}
	reply.Success = true
	return nil
}

type SetModelArgs struct {
	AgentIndex int
	ModelID    string
}

type SetModelReply struct {
	Success bool
	Message string
}

func (s *Service) SetAgentModel(args *SetModelArgs, reply *SetModelReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index out of range")
	}

	s.Agents[args.AgentIndex].ModelID = args.ModelID
	reply.Success = true
	reply.Message = fmt.Sprintf("✓ Model set to %s", args.ModelID)
	s.Agents[args.AgentIndex].Logs += fmt.Sprintf("\n> %s\n", reply.Message)
	return nil
}

type SetConfigArgs struct {
	AgentIndex int
	Thinking   *bool
	Effort     *string
	Verbosity  *string
}

type SetConfigReply struct {
	Success bool
	Message string
}

func (s *Service) SetAgentConfig(args *SetConfigArgs, reply *SetConfigReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index out of range")
	}

	agent := s.Agents[args.AgentIndex]
	if args.Thinking != nil {
		agent.Thinking = *args.Thinking
	}
	if args.Effort != nil {
		agent.Effort = *args.Effort
	}
	if args.Verbosity != nil {
		agent.Verbosity = *args.Verbosity
	}

	reply.Success = true
	reply.Message = "✓ Configuration updated"
	agent.Logs += fmt.Sprintf("\n> %s\n", reply.Message)
	return nil
}

type ListArgs struct{}

type ListReply struct {
	Agents []AgentState
}

func (s *Service) ListAgents(args *ListArgs, reply *ListReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	reply.Agents = make([]AgentState, len(s.Agents))
	for i, ag := range s.Agents {
		reply.Agents[i] = *ag
	}
	return nil
}

type KillArgs struct {
	AgentIndex int
}

type KillReply struct {
	Success bool
	Message string
}

func (s *Service) Kill(args *KillArgs, reply *KillReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index %d out of range", args.AgentIndex)
	}

	name := s.Agents[args.AgentIndex].Name

	// Close the inbox channel so the agent goroutine exits its range loop
	if ch, ok := s.InboxChans[args.AgentIndex]; ok {
		close(ch)
		delete(s.InboxChans, args.AgentIndex)
	}

	// Remove agent from slice
	s.Agents = append(s.Agents[:args.AgentIndex], s.Agents[args.AgentIndex+1:]...)

	// Rebuild inbox channel map with corrected indices
	newInboxChans := make(map[int]chan InboundMessage)
	for i, ch := range s.InboxChans {
		if i > args.AgentIndex {
			newInboxChans[i-1] = ch
		} else {
			newInboxChans[i] = ch
		}
	}
	s.InboxChans = newInboxChans

	reply.Success = true
	reply.Message = "✓ Killed: " + name
	return nil
}
