package viche

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RegisterResponse is what Viche returns after registration
type RegisterResponse struct {
	ID string `json:"id"`
}

// InboxMessage is a message received via WebSocket push
type InboxMessage struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Body string `json:"body"`
	Type string `json:"type"`
}

// Client handles Viche HTTP registration
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "https://viche.ai"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) IsAuthenticated() bool { return c.token != "" }

func (c *Client) RegistryLabel() string {
	if c.token == "" {
		return fmt.Sprintf("%s (public)", c.baseURL)
	}
	short := c.token
	if len(short) > 8 {
		short = short[:8] + "..."
	}
	return fmt.Sprintf("%s (private: %s)", c.baseURL, short)
}

// BaseURL returns the configured base URL
func (c *Client) BaseURL() string { return c.baseURL }

// Token returns the registry token (empty for public)
func (c *Client) Token() string { return c.token }

// Register registers the agent via HTTP and returns its assigned ID
func (c *Client) Register(name string, capabilities []string) (string, error) {
	body := map[string]interface{}{
		"name":         name,
		"capabilities": capabilities,
	}
	if c.token != "" {
		body["registries"] = []string{c.token}
	}

	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/registry/register", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("failed to register with viche: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("viche register failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var reg RegisterResponse
	if err := json.Unmarshal(respBody, &reg); err != nil {
		return "", fmt.Errorf("failed to parse viche response: %w", err)
	}

	return reg.ID, nil
}

// SendMessage sends a fire-and-forget message to another agent via HTTP
func (c *Client) SendMessage(toAgentID, fromAgentID, msgType, body string) error {
	payload := map[string]string{
		"from": fromAgentID,
		"type": msgType,
		"body": body,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/messages/"+toAgentID, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}
