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

	// CloneArgs stores the original hatch parameters for each agent (used for cloning)
	CloneArgs map[int]*CloneArgs
}

type InboundMessage struct {
	From    string // "user" or a viche agent ID
	Content string
}

func NewService() *Service {
	return &Service{
		Agents:     []*AgentState{},
		InboxChans: make(map[int]chan InboundMessage),
		CloneArgs:  make(map[int]*CloneArgs),
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
	WorkDir     string
}

type HatchReply struct {
	Success bool
	Message string
}

type CloneArgs struct {
	Description string
	Registry    string
	ModelID     string
	Thinking    bool
	Effort      string
	Name        string
	Ambient     bool
	WorkDir     string
}

type CloneRequest struct {
	AgentIndex int
}

type CloneResponse struct {
	Success bool
	Message string
	NewID   string
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

	// Store clone args for later use by CloneAgent
	s.CloneArgs[idx] = &CloneArgs{
		Description: args.Description,
		Registry:    args.Registry,
		ModelID:     args.ModelID,
		Thinking:    args.Thinking,
		Effort:      args.Effort,
		Name:        args.Name,
		Ambient:     args.Ambient,
		WorkDir:     args.WorkDir,
	}

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

func (s *Service) CloneAgent(req *CloneRequest, res *CloneResponse) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if req.AgentIndex < 0 || req.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index %d out of range", req.AgentIndex)
	}

	cloneArgs, ok := s.CloneArgs[req.AgentIndex]
	if !ok {
		return fmt.Errorf("no clone args found for agent index %d", req.AgentIndex)
	}

	idx := len(s.Agents)

	cloneName := cloneArgs.Name
	if cloneName == "" {
		cloneName = cloneArgs.Description
	}
	if cloneName == "" {
		cloneName = s.Agents[req.AgentIndex].Name + "-clone"
	} else {
		cloneName = cloneName + "-clone"
	}

	newAgent := &AgentState{
		ID:        fmt.Sprintf("%d", idx+1),
		Name:      cloneName,
		Role:      "auto",
		Ctx:       "0 / 128k",
		State:     "hatching",
		Logs:      "",
		ModelID:   cloneArgs.ModelID,
		Thinking:  cloneArgs.Thinking,
		Effort:    cloneArgs.Effort,
		Verbosity: "verbose",
	}

	s.Agents = append(s.Agents, newAgent)

	inbox := make(chan InboundMessage, 64)
	s.InboxChans[idx] = inbox

	s.CloneArgs[idx] = &CloneArgs{
		Description: cloneArgs.Description,
		Registry:    cloneArgs.Registry,
		ModelID:     cloneArgs.ModelID,
		Thinking:    cloneArgs.Thinking,
		Effort:      cloneArgs.Effort,
		Name:        cloneName,
		Ambient:     cloneArgs.Ambient,
		WorkDir:     cloneArgs.WorkDir,
	}

	if RunEngineHook != nil {
		hatchArgs := &HatchArgs{
			Description: cloneArgs.Description,
			Registry:    cloneArgs.Registry,
			ModelID:     cloneArgs.ModelID,
			Thinking:    cloneArgs.Thinking,
			Effort:      cloneArgs.Effort,
			Name:        cloneName,
			Ambient:     cloneArgs.Ambient,
			WorkDir:     cloneArgs.WorkDir,
		}
		go RunEngineHook(newAgent, func(f func()) {
			s.Mu.Lock()
			defer s.Mu.Unlock()
			f()
		}, hatchArgs, inbox)
	}

	res.Success = true
	res.Message = "Cloned agent: " + s.Agents[req.AgentIndex].Name
	res.NewID = newAgent.ID
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

type ExternalStreamEvent struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	State     string `json:"state"`
	LogLine   string `json:"log_line"`
}

type StreamExternalReply struct {
	Success bool
	Message string
}

func (s *Service) StreamExternalAgent(event *ExternalStreamEvent, reply *StreamExternalReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	var agent *AgentState

	for _, ag := range s.Agents {
		if ag.ID == event.AgentID {
			agent = ag
			break
		}
	}

	if agent == nil {
		agent = &AgentState{
			ID:        event.AgentID,
			Name:      event.AgentName,
			Role:      "external",
			State:     "flying",
			ModelID:   "external",
			Verbosity: "verbose",
		}
		s.Agents = append(s.Agents, agent)
		s.InboxChans[len(s.Agents)-1] = make(chan InboundMessage, 64)
	}

	if event.State != "" {
		agent.State = event.State
	}

	if event.LogLine != "" {
		agent.Logs += event.LogLine + "\n"
	}

	reply.Success = true
	reply.Message = "Event received"
	return nil
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

	// Rebuild clone args map with corrected indices
	newCloneArgs := make(map[int]*CloneArgs)
	for i, ca := range s.CloneArgs {
		if i > args.AgentIndex {
			newCloneArgs[i-1] = ca
		} else {
			newCloneArgs[i] = ca
		}
	}
	s.CloneArgs = newCloneArgs

	reply.Success = true
	reply.Message = "✓ Killed: " + name
	return nil
}
