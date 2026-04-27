package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

func TestCallbackServer_StartStop(t *testing.T) {
	srv := NewCallbackServer(nil, nil)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if port == 0 {
		t.Fatal("expected non-zero port")
	}
	if srv.Port() != port {
		t.Fatalf("Port() returned %d, expected %d", srv.Port(), port)
	}

	url := srv.URL()
	expected := fmt.Sprintf("http://127.0.0.1:%d", port)
	if url != expected {
		t.Fatalf("URL() = %q, expected %q", url, expected)
	}

	srv.Stop()

	_, err = http.Post(fmt.Sprintf("http://127.0.0.1:%d/status", port), "application/json", bytes.NewReader([]byte(`{}`)))
	if err == nil {
		t.Fatal("expected connection refused after Stop")
	}
}

func TestCallbackServer_StatusEndpoint(t *testing.T) {
	var mu sync.Mutex
	var received map[string]interface{}

	srv := NewCallbackServer(
		func(payload map[string]interface{}) {
			mu.Lock()
			received = payload
			mu.Unlock()
		},
		nil,
	)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	body, _ := json.Marshal(map[string]interface{}{
		"state":   "thinking",
		"message": "working on it",
	})

	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/status", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /status failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if received == nil {
		t.Fatal("handler was not called")
	}
	if received["state"] != "thinking" {
		t.Fatalf("expected state 'thinking', got %v", received["state"])
	}
}

func TestCallbackServer_LogEndpoint(t *testing.T) {
	var mu sync.Mutex
	var received map[string]interface{}

	srv := NewCallbackServer(
		nil,
		func(payload map[string]interface{}) {
			mu.Lock()
			received = payload
			mu.Unlock()
		},
	)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	body, _ := json.Marshal(map[string]interface{}{
		"message": "step 3 complete",
		"level":   "info",
	})

	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/log", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /log failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if received == nil {
		t.Fatal("handler was not called")
	}
	if received["message"] != "step 3 complete" {
		t.Fatalf("expected message 'step 3 complete', got %v", received["message"])
	}
}

func TestCallbackServer_MethodNotAllowed(t *testing.T) {
	srv := NewCallbackServer(nil, nil)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_InvalidJSON(t *testing.T) {
	srv := NewCallbackServer(nil, nil)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/status", port),
		"application/json",
		bytes.NewReader([]byte("not json")),
	)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_NilHandlers(t *testing.T) {
	// Nil handlers should still accept requests without panicking
	srv := NewCallbackServer(nil, nil)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	body, _ := json.Marshal(map[string]interface{}{"test": true})

	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%d/status", port),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with nil handler, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_DoubleStop(t *testing.T) {
	srv := NewCallbackServer(nil, nil)
	_, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double stop should not panic
	srv.Stop()
	srv.Stop()
}
