<div align="center">

# TLS Fetch MCP

**Browser-like HTTP requests for AI coding agents — implemented as a secure, local MCP server in Go.**

[![CI](https://img.shields.io/github/actions/workflow/status/JakobAIOdev/tls-fetch-mcp/ci.yml?branch=main&style=flat-square)](https://github.com/JakobAIOdev/tls-fetch-mcp/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![MCP](https://img.shields.io/badge/MCP-stdio-6f42c1?style=flat-square)](https://modelcontextprotocol.io/)
[![License](https://img.shields.io/github/license/JakobAIOdev/tls-fetch-mcp?style=flat-square)](LICENSE)

[Features](#features) · [Quick start](#quick-start) · [Codex setup](#use-with-codex) · [Tool reference](#tool-reference) · [Security](#security-model)

</div>

TLS Fetch MCP gives Codex and other MCP clients a controlled HTTP client with
browser-like TLS fingerprints. It is designed for web development, API
inspection, debugging, and authorized scraping where a regular Go
`net/http.Client` does not accurately resemble a modern browser.

It uses [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) for
TLS and HTTP fingerprinting and the official
[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)
for MCP.

## Features

- **Browser TLS fingerprints** — Chrome, Firefox, Safari, Brave, Opera, mobile
  applications, and every profile exposed by the installed `tls-client`
  version.
- **Intent-aware tools** — read-only GET/HEAD traffic is isolated in
  `tls_get`; POST, PUT, PATCH, DELETE, and OPTIONS live in the explicitly
  mutating `tls_request` tool.
- **Accurate header behavior** — sensible browser headers, matching user-agent
  versions, and optional explicit header ordering.
- **Managed cookie sessions** — warm up regional homepages, preserve cookies
  across multi-step flows, inspect safe session metadata, and automatically
  expire idle sessions.
- **Large-response workflow** — keep a bounded response out of the agent
  context, receive a short-lived `response_id`, then search it or read it in
  byte ranges.
- **Proxy support** — optional HTTP, HTTPS, SOCKS5, and SOCKS5H proxies.
- **Structured responses** — status, HTTP version, final URL, redirect history,
  safe headers, content length, returned bytes, body encoding, timing,
  truncation state, selected profile, and session metadata.
- **Secret-safe output** — `Set-Cookie`, authorization, and proxy
  authentication response headers are redacted. Cookie values are never
  returned; only cookie names and counts are exposed.
- **Binary-safe output** — non-UTF-8 response bodies are returned as Base64.
- **Resource controls** — server-side timeout, response-size, session-count,
  response-handle count, TTL, read-window, and redirect limits.
- **Secure by default** — private networks, loopback, link-local ranges,
  multicast, unspecified addresses, and caller-supplied proxies are blocked
  unless the operator explicitly enables them.
- **DNS-rebinding protection** — addresses are checked before the request and
  again immediately before the network connection.
- **Single local binary** — no browser, Node.js runtime, or background daemon
  required.

## How it works

```mermaid
flowchart LR
    A["Codex / MCP client"] -->|stdio| B["TLS Fetch MCP"]
    B --> C["URL and SSRF policy"]
    C --> D["Browser profile + headers"]
    D --> E["tls-client"]
    E -->|HTTP/1.1 or HTTP/2| F["Target website / API"]
    E <--> G["In-memory cookie session"]
    B <--> H["Bounded response store"]
```

The MCP client starts the server as a local `stdio` process. Every requested
URL passes through the host allowlist and IP policy. The server then builds a
fingerprinted client, performs the request, redacts secret response headers,
limits the body, and returns a typed MCP result. Cookie sessions and stored
responses exist only in memory and expire automatically.

## Quick start

### Requirements

- Go 1.25 or newer
- An MCP client with local `stdio` server support

### Build from source

```bash
git clone https://github.com/JakobAIOdev/tls-fetch-mcp.git
cd tls-fetch-mcp
make build
```

The binary is written to `bin/tls-fetch-mcp`.

You can also run it directly:

```bash
go run ./cmd/tls-fetch-mcp
```

Check a built binary:

```bash
./bin/tls-fetch-mcp --version
```

Once the repository is published, installation with Go is also supported:

```bash
go install github.com/JakobAIOdev/tls-fetch-mcp/cmd/tls-fetch-mcp@latest
```

## Use with Codex

Codex can load an MCP server globally from `~/.codex/config.toml` or only for a
trusted repository from `<project>/.codex/config.toml`.

### Project-scoped configuration

Create `.codex/config.toml` in the project where Codex should use the server:

```toml
[mcp_servers.tls_fetch]
command = "/absolute/path/to/tls-fetch-mcp"
enabled = true
required = true
enabled_tools = [
  "tls_get",
  "tls_request",
  "tls_profiles",
  "tls_session_warmup",
  "tls_session_info",
  "tls_session_clear",
  "tls_response_read",
  "tls_response_search",
]
startup_timeout_sec = 10
tool_timeout_sec = 120
default_tools_approval_mode = "prompt"

[mcp_servers.tls_fetch.tools.tls_get]
approval_mode = "auto"

[mcp_servers.tls_fetch.tools.tls_profiles]
approval_mode = "auto"

[mcp_servers.tls_fetch.tools.tls_session_info]
approval_mode = "auto"

[mcp_servers.tls_fetch.tools.tls_response_read]
approval_mode = "auto"

[mcp_servers.tls_fetch.tools.tls_response_search]
approval_mode = "auto"
```

For local development targets such as `localhost:3000`, add:

```toml
[mcp_servers.tls_fetch.env]
MCP_TLS_FETCH_ALLOW_PRIVATE = "true"
```

Restart Codex or open a new task after changing the configuration. Use `/mcp`
to verify that the tools are available. The legacy `tls_fetch` compatibility
tool is intentionally omitted here so Codex naturally selects `tls_get` or
`tls_request`.

### Global CLI configuration

```bash
codex mcp add tls-fetch \
  -- /absolute/path/to/tls-fetch-mcp
```

To permit local development targets globally:

```bash
codex mcp add tls-fetch \
  --env MCP_TLS_FETCH_ALLOW_PRIVATE=true \
  -- /absolute/path/to/tls-fetch-mcp
```

Verify the configuration:

```bash
codex mcp list
```

## Tool reference

### `tls_get`

Sends a read-only, fingerprinted GET or HEAD request. This is the default tool
for inspecting websites, discovering JSON endpoints, and authorized scraping.

| Input | Type | Default | Description |
| --- | --- | --- | --- |
| `url` | string | required | Absolute HTTP or HTTPS URL |
| `method` | string | `GET` | GET or HEAD |
| `headers` | object | browser defaults | Request header name-value pairs |
| `header_order` | string[] | browser-like order | Explicit lower-case header order |
| `profile` | string | `chrome_146` | TLS profile returned by `tls_profiles` |
| `follow_redirects` | boolean | `true` | Follow up to ten policy-checked redirects |
| `timeout_seconds` | integer | `30` | Whole-request timeout, capped by server configuration |
| `max_response_bytes` | integer | `524288` | Returned or stored body limit, capped by server configuration |
| `proxy_url` | string | empty | Optional proxy; requires operator opt-in |
| `session_id` | string | empty | Optional cookie-session identifier |
| `include_body` | boolean | `true` | Include the bounded body directly in the result |
| `store_response` | boolean | `false` | Return a temporary `response_id` for read/search tools |

Example prompt:

```text
Use tls_get with the chrome_146 profile to inspect https://example.com.
Return the status, final URL, content type, and HTML title.
```

For a large response:

```text
Fetch the catalog with tls_get using store_response=true and
include_body=false. Search the stored response for "pagination", then read
only the relevant byte range.
```

### `tls_request`

Sends POST, PUT, PATCH, DELETE, or OPTIONS requests. It accepts the same
options as `tls_get`, plus `body`. GET and HEAD are deliberately rejected so
MCP clients can apply separate approval rules to read and write traffic.

```text
Use tls_request to POST {"query":"mcp"} to
https://example.com/api/search. Set Content-Type to application/json.
```

### `tls_fetch`

Compatibility alias for existing clients. It still accepts every supported
method, but new integrations should prefer `tls_get` and `tls_request`.

### `tls_profiles`

Returns the default profile and every fingerprint supported by the pinned
`tls-client` version. Profile availability follows the dependency version
rather than a hardcoded MCP schema enum.

### `tls_session_warmup`

Fetches a bootstrap URL without returning its body and retains cookies in the
provided `session_id`. This is useful when an API first returns `401`, but the
same request succeeds after visiting the regional homepage.

```text
1. Warm up https://www.example.de/ as session "catalog-de".
2. Call tls_get for the catalog endpoint with session_id "catalog-de".
3. Inspect tls_session_info if authentication still fails.
4. Clear the session when the flow is finished.
```

### `tls_session_info`

Returns existence, created/last-used/expiry timestamps, cookie count, and
cookie names. Cookie values are never exposed.

### `tls_session_clear`

Deletes one in-memory cookie session. Session IDs may contain letters, digits,
dots, underscores, and hyphens and must not exceed 128 characters.

### `tls_response_search`

Searches literal UTF-8 text in a stored response. It returns byte offsets and
compact context windows. Matching is ASCII case-insensitive by default and can
be made case-sensitive.

### `tls_response_read`

Reads a stored response from a zero-based byte `offset`, up to `max_bytes`.
The result includes `next_offset`, `total_bytes`, and `eof`, so callers can
page without placing the entire document in model context. Non-UTF-8 chunks
are Base64-encoded.

## Configuration

All server-level behavior is configured through environment variables. Tool
callers may choose stricter request limits but cannot exceed these values.

| Variable | Default | Description |
| --- | ---: | --- |
| `MCP_TLS_FETCH_ALLOW_PRIVATE` | `false` | Allow private, loopback, and link-local targets |
| `MCP_TLS_FETCH_ALLOW_PROXY` | `false` | Allow callers to provide `proxy_url` |
| `MCP_TLS_FETCH_ALLOWED_HOSTS` | empty | Comma-separated host allowlist; supports `*.example.com` |
| `MCP_TLS_FETCH_MAX_RESPONSE_BYTES` | `524288` | Maximum returned response body size |
| `MCP_TLS_FETCH_DEFAULT_TIMEOUT_SECONDS` | `30` | Default whole-request timeout |
| `MCP_TLS_FETCH_MAX_TIMEOUT_SECONDS` | `120` | Maximum caller-selectable timeout |
| `MCP_TLS_FETCH_MAX_SESSIONS` | `64` | Maximum in-memory cookie sessions |
| `MCP_TLS_FETCH_SESSION_TTL_SECONDS` | `1800` | Sliding idle TTL for cookie sessions |
| `MCP_TLS_FETCH_MAX_STORED_RESPONSES` | `32` | Maximum temporary response handles |
| `MCP_TLS_FETCH_RESPONSE_TTL_SECONDS` | `300` | TTL for temporary response handles |
| `MCP_TLS_FETCH_MAX_RESPONSE_READ_BYTES` | `131072` | Maximum bytes returned by one response-read call |

Example: restrict the server to one domain and increase the response limit to
2 MiB:

```bash
MCP_TLS_FETCH_ALLOWED_HOSTS='example.com,*.example.com' \
MCP_TLS_FETCH_MAX_RESPONSE_BYTES=2097152 \
./bin/tls-fetch-mcp
```

## Security model

The server treats every tool argument as untrusted.

- Only absolute `http` and `https` target URLs are accepted.
- URL-embedded credentials are rejected; authentication belongs in headers.
- Hop-by-hop and transport-managed request headers cannot be overridden.
- Host allowlists are checked before DNS resolution.
- Every resolved IP address must pass the network policy.
- Direct connections resolve and validate the address again at dial time.
- Redirect destinations pass through the same URL and IP policy.
- Unspecified and multicast destinations remain blocked even when private
  targets are enabled.
- Proxy use requires explicit operator opt-in.
- Sensitive response headers are removed before MCP serialization.
- Cookie values remain inside the in-memory jar and are never returned.
- Response bodies, response-handle counts, read windows, session counts, and
  redirect counts are bounded.
- Sessions and stored responses expire automatically.

When a proxy is enabled, the proxy performs the final target connection and may
resolve DNS differently from the MCP host. Only configure proxies you trust.

For the complete threat model and reporting process, see
[`SECURITY.md`](SECURITY.md).

## Important limitations

- TLS fingerprinting is not full browser automation. This server does not run
  JavaScript, render a DOM, solve CAPTCHAs, or reproduce browser behavior.
- HTTP/3 is intentionally disabled so the direct connection can always pass
  through the dial-time SSRF policy. HTTP/1.1 and HTTP/2 fingerprinting remain
  active.
- Cookie sessions and response handles live only in memory, expire by TTL, and
  disappear when the server exits.
- Stored responses contain only the already-bounded response bytes.
- A successful request does not grant permission to scrape a service or bypass
  its access controls.

For JavaScript-heavy flows, pair this server with a browser automation MCP
instead of trying to turn HTTP requests into a browser.

## Development

Common commands:

```bash
make format
make test
make vet
make build
make check
```

Run the optional live TLS integration test:

```bash
TLS_FETCH_INTEGRATION_URL=https://example.com make test-integration
```

Project layout:

```text
cmd/tls-fetch-mcp/   MCP server entry point and tool registration
internal/fetch/      Fetch engine, URL policy, configuration, and tests
.github/             CI, dependency updates, and contribution templates
```

Pull requests run formatting checks, tests, `go vet`, and a clean build on
Linux, macOS, and Windows. Tags matching `v*` create cross-platform release
archives and SHA-256 checksums.

## Roadmap

- Configurable profile aliases and default profiles
- Optional HTML selectors and JSON-path extraction helpers
- HTTP/3 support with equivalent connection-policy enforcement

Contributions and focused feature proposals are welcome. Read
[`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a pull request.

## Responsible use

Only access systems you are authorized to test or automate. Respect applicable
laws, terms of service, robots directives, privacy requirements, and rate
limits. Do not use this project to evade access controls or cause harm.

## License

Released under the [MIT License](LICENSE).
