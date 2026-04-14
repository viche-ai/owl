package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// CallbackHandler is a function that processes a JSON payload from the harness.
type CallbackHandler func(payload map[string]interface{})

// CallbackServer runs a localhost HTTP server that harness subprocesses can
// POST to for reporting status, logs, and other structured events back to Owl.
type CallbackServer struct {
	mu       sync.Mutex
	port     int
	listener net.Listener
	srv      *http.Server

	onStatus CallbackHandler
	onLog    CallbackHandler
}

// NewCallbackServer creates a callback server with the given handlers.
// Either handler may be nil (calls to that endpoint will be accepted but ignored).
func NewCallbackServer(onStatus, onLog CallbackHandler) *CallbackServer {
	return &CallbackServer{
		onStatus: onStatus,
		onLog:    onLog,
	}
}

// Start binds to an ephemeral localhost port and begins serving.
// Returns the port number so it can be injected into the harness environment.
func (s *CallbackServer) Start() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("callback server listen: %w", err)
	}

	s.mu.Lock()
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleEndpoint(s.onStatus))
	mux.HandleFunc("/log", s.handleEndpoint(s.onLog))

	s.srv = &http.Server{Handler: mux}

	go func() {
		_ = s.srv.Serve(ln) // best-effort; ErrServerClosed is expected on Stop()
	}()

	return s.port, nil
}

// Port returns the port the server is listening on, or 0 if not started.
func (s *CallbackServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Stop gracefully shuts down the callback server.
func (s *CallbackServer) Stop() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

// URL returns the full base URL of the callback server (e.g. "http://127.0.0.1:12345").
func (s *CallbackServer) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *CallbackServer) handleEndpoint(handler CallbackHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if handler != nil {
			handler(payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
