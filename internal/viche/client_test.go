package viche

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_BasicMethods(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		token       string
		expectAuth  bool
		expectURL   string
		expectLabel string
	}{
		{
			name:        "public client",
			baseURL:     "",
			token:       "",
			expectAuth:  false,
			expectURL:   "https://viche.ai",
			expectLabel: "https://viche.ai (public)",
		},
		{
			name:        "private client short token",
			baseURL:     "http://localhost:8080",
			token:       "short",
			expectAuth:  true,
			expectURL:   "http://localhost:8080",
			expectLabel: "http://localhost:8080 (private: short)",
		},
		{
			name:        "private client long token",
			baseURL:     "http://localhost:8080/", // tests trailing slash removal
			token:       "long-token-value-123456",
			expectAuth:  true,
			expectURL:   "http://localhost:8080",
			expectLabel: "http://localhost:8080 (private: long-tok...)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.baseURL, tc.token)
			if c.IsAuthenticated() != tc.expectAuth {
				t.Errorf("expected IsAuthenticated %v, got %v", tc.expectAuth, c.IsAuthenticated())
			}
			if c.BaseURL() != tc.expectURL {
				t.Errorf("expected BaseURL %q, got %q", tc.expectURL, c.BaseURL())
			}
			if c.RegistryLabel() != tc.expectLabel {
				t.Errorf("expected RegistryLabel %q, got %q", tc.expectLabel, c.RegistryLabel())
			}
			if c.Token() != tc.token {
				t.Errorf("expected Token %q, got %q", tc.token, c.Token())
			}
		})
	}
}

func TestClient_Register(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/registry/register" {
			t.Errorf("expected path /registry/register, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected method POST, got %s", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["name"] != "test-agent" {
			t.Errorf("expected name 'test-agent', got %v", body["name"])
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RegisterResponse{ID: "agent-123"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "secret")
	id, err := c.Register("test-agent", []string{"testing"})

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if id != "agent-123" {
		t.Errorf("expected ID 'agent-123', got %q", id)
	}
}

func TestClient_SendMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages/agent-dest" {
			t.Errorf("expected path /messages/agent-dest, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected method POST, got %s", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["from"] != "agent-src" {
			t.Errorf("expected from 'agent-src', got %v", body["from"])
		}
		if body["type"] != "text" {
			t.Errorf("expected type 'text', got %v", body["type"])
		}
		if body["body"] != "hello" {
			t.Errorf("expected body 'hello', got %v", body["body"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "secret")
	err := c.SendMessage("agent-dest", "agent-src", "text", "hello")

	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}
