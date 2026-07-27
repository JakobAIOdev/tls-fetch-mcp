package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/JakobAIOdev/tls-fetch-mcp/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("tls-fetch-mcp %s\n", version)
		return
	}

	cfg, err := fetch.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	fetcher := fetch.New(cfg)
	server := newServer(fetcher)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func newServer(fetcher *fetch.Fetcher) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "tls-fetch-mcp", Version: version},
		&mcp.ServerOptions{
			Instructions: "Use tls_get for authorized read-only web inspection and scraping with browser-like TLS fingerprints. " +
				"Use tls_request only when the task explicitly requires POST, PUT, PATCH, DELETE, or OPTIONS. " +
				"For cookie-gated sites, call tls_session_warmup once and reuse its session_id. " +
				"For large or structured bodies, set store_response=true and include_body=false, then use tls_response_extract, tls_response_search or tls_response_read. " +
				"Sensitive response headers are redacted. Respect site policies and rate limits. Private targets and proxies are disabled unless explicitly configured.",
		},
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:  "tls_get",
		Title: "TLS Get",
		Description: "Send an authorized GET or HEAD request with a browser-like TLS fingerprint. " +
			"Use this as the default tool for web inspection, API discovery and scraping. " +
			"Supports cookie sessions, redirects, bounded bodies and temporary response handles for extraction, search or ranged reads.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(true),
			ReadOnlyHint:  true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input fetch.GetInput) (*mcp.CallToolResult, fetch.Output, error) {
		output, err := fetcher.DoGet(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "tls_request",
		Title: "TLS Request",
		Description: "Send an authorized mutating HTTP request (POST, PUT, PATCH, DELETE or OPTIONS) with a browser-like TLS fingerprint. " +
			"Use tls_get for GET and HEAD. Supports request bodies, cookie sessions and temporary response handles for extraction, search or ranged reads.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPointer(true),
			OpenWorldHint:   boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input fetch.RequestInput) (*mcp.CallToolResult, fetch.Output, error) {
		output, err := fetcher.DoRequest(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "tls_fetch",
		Title: "TLS Fetch (Compatibility)",
		Description: "Compatibility tool for existing clients. Send any supported HTTP method with a browser-like TLS fingerprint. " +
			"New integrations should use tls_get for GET/HEAD and tls_request for mutating methods. " +
			"Supports browser profiles, custom headers, redirects, response limits, proxies and cookie sessions. " +
			"Private and loopback targets are blocked unless enabled by server configuration.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPointer(true),
			OpenWorldHint:   boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input fetch.Input) (*mcp.CallToolResult, fetch.Output, error) {
		output, err := fetcher.Do(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_profiles",
		Title:       "TLS Profiles",
		Description: "List the browser TLS fingerprints accepted by the TLS request tools.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(false),
			ReadOnlyHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ fetch.ProfilesInput) (*mcp.CallToolResult, fetch.ProfilesOutput, error) {
		return nil, fetch.ProfilesOutput{
			Default:  fetch.DefaultProfile,
			Profiles: fetch.ProfileNames(),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "tls_session_warmup",
		Title: "Warm Up TLS Session",
		Description: "GET a regional homepage or bootstrap URL without returning its body, then retain cookies in a named in-memory session. " +
			"Use before a cookie-gated catalog or API request and reuse the same session_id.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(true),
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input fetch.SessionWarmupInput) (*mcp.CallToolResult, fetch.Output, error) {
		output, err := fetcher.WarmupSession(ctx, input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_session_info",
		Title:       "TLS Session Info",
		Description: "Inspect the existence, expiry and non-secret cookie metadata of one in-memory TLS session.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(false),
			ReadOnlyHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input fetch.SessionInfoInput) (*mcp.CallToolResult, fetch.SessionInfoOutput, error) {
		output, err := fetcher.SessionInfo(input.SessionID)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_session_clear",
		Title:       "Clear TLS Session",
		Description: "Delete one in-memory cookie session created by the TLS request tools.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPointer(true),
			OpenWorldHint:   boolPointer(false),
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input fetch.SessionClearInput) (*mcp.CallToolResult, fetch.SessionClearOutput, error) {
		cleared, err := fetcher.ClearSession(input.SessionID)
		return nil, fetch.SessionClearOutput{
			SessionID: input.SessionID,
			Cleared:   cleared,
		}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_response_read",
		Title:       "Read Stored TLS Response",
		Description: "Read a byte range from a temporary bounded response created with store_response=true. Handles expire automatically.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(false),
			ReadOnlyHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input fetch.ResponseReadInput) (*mcp.CallToolResult, fetch.ResponseReadOutput, error) {
		output, err := fetcher.ReadResponse(input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_response_extract",
		Title:       "Extract Stored TLS Response",
		Description: "Extract compact structured values from a temporary stored response. Supports CSS selectors for HTML and RFC 9535 JSONPath for JSON, with bounded query counts, result counts and serialized output.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(false),
			ReadOnlyHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input fetch.ResponseExtractInput) (*mcp.CallToolResult, fetch.ResponseExtractOutput, error) {
		output, err := fetcher.ExtractResponse(input)
		return nil, output, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tls_response_search",
		Title:       "Search Stored TLS Response",
		Description: "Search literal text in a temporary UTF-8 response and return compact context windows instead of the entire body.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPointer(false),
			ReadOnlyHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input fetch.ResponseSearchInput) (*mcp.CallToolResult, fetch.ResponseSearchOutput, error) {
		output, err := fetcher.SearchResponse(input)
		return nil, output, err
	})

	return server
}

func boolPointer(value bool) *bool {
	return &value
}
