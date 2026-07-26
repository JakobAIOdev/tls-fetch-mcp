package main

import (
	"context"
	"slices"
	"testing"

	"github.com/JakobAIOdev/tls-fetch-mcp/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesExpectedTools(t *testing.T) {
	ctx := context.Background()
	server := newServer(fetch.New(fetch.Config{
		MaxResponseBytes: 1024,
		DefaultTimeout:   5,
		MaxTimeout:       10,
		MaxSessions:      8,
	}))
	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.1.0"},
		nil,
	)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
	}
	for _, name := range []string{"tls_fetch", "tls_profiles", "tls_session_clear"} {
		if !slices.Contains(names, name) {
			t.Fatalf("tool names = %v, missing %s", names, name)
		}
	}

	profilesResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "tls_profiles",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(tls_profiles) error = %v", err)
	}
	if profilesResult.IsError || profilesResult.StructuredContent == nil {
		t.Fatalf("CallTool(tls_profiles) = %+v, want structured success", profilesResult)
	}

	clearResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "tls_session_clear",
		Arguments: map[string]any{
			"session_id": "missing-session",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(tls_session_clear) error = %v", err)
	}
	if clearResult.IsError || clearResult.StructuredContent == nil {
		t.Fatalf("CallTool(tls_session_clear) = %+v, want structured success", clearResult)
	}
}
