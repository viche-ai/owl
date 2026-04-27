package testutil

import (
	"fmt"
	"sync"

	"github.com/viche-ai/owl/internal/viche"
)

type FakeViche struct {
	mu sync.Mutex

	BaseURLValue  string
	TokenValue    string
	RegistryValue string
	AgentID       string
	Authenticated bool

	RegisterError error
	ConnectError  error
	PushError     error

	RegisterCalls []FakeRegisterCall
	PushCalls     []FakePushCall
	Inbox         chan viche.InboxMessage

	onMessage func(viche.InboxMessage)
	done      chan struct{}
	once      sync.Once
}

type FakeRegisterCall struct {
	Name         string
	Capabilities []string
}

type FakePushCall struct {
	Event   string
	Payload map[string]interface{}
}

func (f *FakeViche) IsAuthenticated() bool {
	return f.Authenticated || f.TokenValue != ""
}

func (f *FakeViche) RegistryLabel() string {
	if f.RegistryValue != "" {
		return f.RegistryValue
	}
	if f.BaseURLValue == "" {
		return "fake-registry"
	}
	return f.BaseURLValue
}

func (f *FakeViche) Register(name string, capabilities []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.RegisterCalls = append(f.RegisterCalls, FakeRegisterCall{
		Name:         name,
		Capabilities: append([]string(nil), capabilities...),
	})
	if f.RegisterError != nil {
		return "", f.RegisterError
	}
	if f.AgentID == "" {
		f.AgentID = "fake-agent-id"
	}
	return f.AgentID, nil
}

func (f *FakeViche) BaseURL() string {
	if f.BaseURLValue == "" {
		return "https://fake-viche.local"
	}
	return f.BaseURLValue
}

func (f *FakeViche) Token() string {
	return f.TokenValue
}

func (f *FakeViche) Connect() error {
	if f.ConnectError != nil {
		return f.ConnectError
	}
	f.mu.Lock()
	if f.done == nil {
		f.done = make(chan struct{})
	}
	inbox := f.Inbox
	onMessage := f.onMessage
	done := f.done
	f.mu.Unlock()

	if inbox != nil && onMessage != nil {
		go func() {
			for {
				select {
				case <-done:
					return
				case msg, ok := <-inbox:
					if !ok {
						return
					}
					onMessage(msg)
				}
			}
		}()
	}
	return nil
}

func (f *FakeViche) Push(event string, payload map[string]interface{}) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	copyPayload := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		copyPayload[k] = v
	}
	f.PushCalls = append(f.PushCalls, FakePushCall{
		Event:   event,
		Payload: copyPayload,
	})

	if f.PushError != nil {
		return nil, f.PushError
	}

	switch event {
	case "discover":
		return map[string]interface{}{
			"agents": []interface{}{
				map[string]interface{}{
					"id":           "fake-target",
					"name":         "fake-target",
					"capabilities": []interface{}{"owl-agent"},
				},
			},
		}, nil
	case "send_message":
		return map[string]interface{}{"status": "ok"}, nil
	default:
		return map[string]interface{}{"status": "ok"}, nil
	}
}

func (f *FakeViche) SetOnMessage(fn func(viche.InboxMessage)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onMessage = fn
}

func (f *FakeViche) Close() {
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()
	if done == nil {
		return
	}
	f.once.Do(func() {
		close(done)
	})
}

func (f *FakeViche) SendInbound(msg viche.InboxMessage) error {
	f.mu.Lock()
	inbox := f.Inbox
	done := f.done
	f.mu.Unlock()
	if inbox == nil {
		return fmt.Errorf("fake viche inbox is not configured")
	}
	select {
	case <-done:
		return fmt.Errorf("fake viche channel is closed")
	default:
	}
	inbox <- msg
	return nil
}
