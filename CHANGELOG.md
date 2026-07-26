# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
