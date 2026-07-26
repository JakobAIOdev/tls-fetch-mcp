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
			Instructions: "Use tls_fetch for authorized HTTP inspection and scraping with browser-like TLS fingerprints. " +
				"Prefer GET/HEAD for discovery, respect site policies and rate limits, and use a session_id only when cookie continuity is required. " +
				"Private targets and caller-supplied proxies are disabled unless the server operator explicitly enables them.",
		},
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:  "tls_fetch",
		Title: "TLS Fetch",
		Description: "Send an HTTP request with a browser-like TLS fingerprint. " +
			"Supports browser profiles, custom headers, redirects, response limits, proxies and cookie sessions. " +
			"Useful for web development, API inspection and authorized scraping. " +
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
		Description: "List the browser TLS fingerprints accepted by tls_fetch.",
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
		Name:        "tls_session_clear",
		Title:       "Clear TLS Session",
		Description: "Delete one in-memory cookie session created by tls_fetch.",
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

	return server
}

func boolPointer(value bool) *bool {
	return &value
}
