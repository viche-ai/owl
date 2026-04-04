package viche

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type phoenixMsg [5]interface{}

func newPhoenixMsg(joinRef, ref, topic, event string, payload interface{}) phoenixMsg {
	return phoenixMsg{joinRef, ref, topic, event, payload}
}

type pendingReply struct {
	ch chan map[string]interface{}
}

type Channel struct {
	conn    *websocket.Conn
	agentID string
	token   string
	baseURL string
	writeMu sync.Mutex
	ref     int64

	pending   map[string]*pendingReply
	pendingMu sync.Mutex

	joined    chan struct{} // closed once channel join is confirmed
	joinRef   string
	OnMessage func(msg InboxMessage)
}

func NewChannel(baseURL, agentID, token string) *Channel {
	return &Channel{
		agentID: agentID,
		token:   token,
		baseURL: baseURL,
		pending: make(map[string]*pendingReply),
		joined:  make(chan struct{}),
	}
}

func (c *Channel) wsURL() string {
	wsBase := strings.Replace(c.baseURL, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	return wsBase + "/agent/websocket/websocket"
}

func (c *Channel) nextRef() string {
	r := atomic.AddInt64(&c.ref, 1)
	return fmt.Sprintf("%d", r)
}

func (c *Channel) writeMsg(msg phoenixMsg) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, b)
}

// Connect dials the WebSocket, joins the agent channel, and waits for confirmation.
func (c *Channel) Connect() error {
	u, err := url.Parse(c.wsURL())
	if err != nil {
		return fmt.Errorf("invalid ws url: %w", err)
	}
	q := u.Query()
	q.Set("agent_id", c.agentID)
	q.Set("vsn", "2.0.0")
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	c.conn = conn

	// Start reader FIRST so we can catch the join reply
	go c.readLoop()

	// Send join
	c.joinRef = fmt.Sprintf("%d", rand.Intn(10000))
	joinRef := c.joinRef
	joinTopic := "agent:" + c.agentID
	if err := c.writeMsg(newPhoenixMsg(joinRef, c.nextRef(), joinTopic, "phx_join", map[string]interface{}{})); err != nil {
		return fmt.Errorf("channel join write failed: %w", err)
	}

	// Also join registry channel if token set
	if c.token != "" {
		_ = c.writeMsg(newPhoenixMsg(joinRef, c.nextRef(), "registry:"+c.token, "phx_join", map[string]interface{}{}))
	}

	// Start heartbeat
	go c.heartbeatLoop()

	// Wait for join confirmation (or timeout)
	select {
	case <-c.joined:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("channel join timeout")
	}
}

// Push sends an event on the agent channel and waits for a reply.
func (c *Channel) Push(event string, payload map[string]interface{}) (map[string]interface{}, error) {
	// Wait for channel to be joined
	select {
	case <-c.joined:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("channel not joined")
	}

	ref := c.nextRef()
	replyCh := make(chan map[string]interface{}, 1)

	c.pendingMu.Lock()
	c.pending[ref] = &pendingReply{ch: replyCh}
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, ref)
		c.pendingMu.Unlock()
	}()

	topic := "agent:" + c.agentID
	if err := c.writeMsg(newPhoenixMsg(c.joinRef, ref, topic, event, payload)); err != nil {
		return nil, err
	}

	select {
	case resp := <-replyCh:
		return resp, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("push timeout for event %s", event)
	}
}

func (c *Channel) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if c.conn == nil {
			return
		}
		_ = c.writeMsg(newPhoenixMsg("", c.nextRef(), "phoenix", "heartbeat", map[string]interface{}{}))
	}
}

func (c *Channel) readLoop() {
	for {
		if c.conn == nil {
			return
		}

		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg phoenixMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if len(msg) < 5 {
			continue
		}

		ref, _ := msg[1].(string)
		event, _ := msg[3].(string)

		switch event {
		case "phx_reply":
			payload, _ := msg[4].(map[string]interface{})
			status, _ := payload["status"].(string)
			response, _ := payload["response"].(map[string]interface{})

			// Check if this is the join reply
			topic, _ := msg[2].(string)
			if topic == "agent:"+c.agentID && status == "ok" {
				select {
				case <-c.joined:
					// already closed
				default:
					close(c.joined)
				}
			}

			// Route to pending Push() calls
			if ref != "" {
				c.pendingMu.Lock()
				if p, ok := c.pending[ref]; ok {
					if response != nil {
						p.ch <- response
					} else {
						p.ch <- payload
					}
				}
				c.pendingMu.Unlock()
			}

		case "new_message":
			if payload, ok := msg[4].(map[string]interface{}); ok {
				inboxMsg := InboxMessage{}
				if id, ok := payload["id"].(string); ok {
					inboxMsg.ID = id
				}
				if from, ok := payload["from"].(string); ok {
					inboxMsg.From = from
				}
				if body, ok := payload["body"].(string); ok {
					inboxMsg.Body = body
				}
				if typ, ok := payload["type"].(string); ok {
					inboxMsg.Type = typ
				}
				if c.OnMessage != nil {
					c.OnMessage(inboxMsg)
				}
			}

		case "phx_error":
			// Channel error — connection will be cleaned up
			return
		}
	}
}

// Close gracefully closes the WebSocket connection
func (c *Channel) Close() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn != nil {
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = c.conn.Close()
		c.conn = nil
	}
}
