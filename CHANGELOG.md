# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Dedicated `tls_get` and `tls_request` tools with read/write MCP annotations
- `tls_session_warmup` and `tls_session_info` tools
- Sliding cookie-session TTLs and safe cookie name/count metadata
- Temporary bounded response handles with `tls_response_read` and
  `tls_response_search`
- `tls_response_extract` for named CSS queries over HTML and RFC 9535 JSONPath
  queries over JSON, including HTML text/markup/attribute modes and relative
  URL resolution
- Redirect history, HTTP version, content length, and byte-count metadata
- Optional inline-body suppression with `include_body=false`
- Response-header redaction for cookies and authentication secrets
- Server limits for session TTL, response TTL, stored response count, and
  response read windows

- Browser-like TLS requests through `bogdanfinn/tls-client`
- Official Go MCP SDK integration over `stdio`
- `tls_fetch`, `tls_profiles`, and `tls_session_clear` tools
- Stateful in-memory cookie sessions
- HTTP, HTTPS, SOCKS5, and SOCKS5H proxy support with operator opt-in
- Structured and binary-safe response output
- Host allowlists, SSRF protection, redirect validation, and DNS-rebinding
  protection
- Server-side timeout, response-size, and session-count limits
- Cross-platform CI and tagged release automation

### Compatibility

- `tls_fetch` remains available for existing MCP configurations
