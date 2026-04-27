package llm

import (
	"context"
	"strings"
	"testing"
)

type stubProvider struct {
	name string
}

func (s stubProvider) ChatStream(_ context.Context, _ string, _ []Message) (<-chan StreamEvent, error) {
	return nil, nil
}

func (s stubProvider) ChatStreamWithTools(_ context.Context, _ string, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, nil
}

func (s stubProvider) Name() string {
	return s.name
}

func TestRouterKnownProvider(t *testing.T) {
	provider := stubProvider{name: "anthropic"}
	router := &Router{
		providers: map[string]Provider{
			"anthropic": provider,
		},
	}

	gotProvider, model, err := router.Resolve("anthropic/claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if gotProvider != provider {
		t.Fatalf("expected provider %#v, got %#v", provider, gotProvider)
	}
	if model != "claude-sonnet-4-6" {
		t.Fatalf("expected model claude-sonnet-4-6, got %q", model)
	}
}

func TestRouterUnknownProvider(t *testing.T) {
	router := &Router{
		providers: map[string]Provider{
			"anthropic": stubProvider{name: "anthropic"},
		},
	}

	_, _, err := router.Resolve("unknown/foo")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), `no provider configured for "unknown"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouterNoPrefix(t *testing.T) {
	router := &Router{
		providers: map[string]Provider{
			"anthropic": stubProvider{name: "anthropic"},
		},
	}

	_, _, err := router.Resolve("claude-sonnet-4-6")
	if err == nil {
		t.Fatal("expected error for missing provider prefix")
	}
	if !strings.Contains(err.Error(), "expected 'provider/model'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
