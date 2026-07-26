# Contributing

Thank you for improving TLS Fetch MCP. Focused bug fixes, security hardening,
documentation improvements, tests, and well-scoped features are welcome.

## Before you start

- Search existing issues before opening a new one.
- For larger behavior changes, open a feature request before investing in an
  implementation.
- Report security vulnerabilities privately as described in
  [`SECURITY.md`](SECURITY.md).
- Only use test targets that you own or are authorized to access.

## Development setup

Requirements:

- Go 1.24.1 or newer
- GNU Make or a compatible `make` implementation (optional)

Clone the repository and download dependencies:

```bash
git clone https://github.com/JakobAIOdev/tls-fetch-mcp.git
cd tls-fetch-mcp
go mod download
```

Run the complete local verification:

```bash
make check
```

Equivalent commands:

```bash
gofmt -w cmd/tls-fetch-mcp/*.go internal/fetch/*.go
go test ./...
go vet ./...
go build ./cmd/tls-fetch-mcp
```

The live TLS integration test is opt-in:

```bash
TLS_FETCH_INTEGRATION_URL=https://example.com make test-integration
```

Never point automated integration tests at a third-party service without
permission.

## Pull requests

1. Create a focused branch from `main`.
2. Add or update tests for behavior changes.
3. Update the README and changelog when user-facing behavior changes.
4. Run `make check`.
5. Open a pull request with a clear problem statement and verification notes.

Please keep unrelated refactors out of feature and bug-fix pull requests.

## Code guidelines

- Prefer standard-library solutions where they keep the implementation clear.
- Keep the MCP input and output schemas backward compatible where possible.
- Treat URLs, headers, proxy settings, session IDs, and remote content as
  untrusted input.
- Preserve bounded timeouts, response sizes, redirects, and session counts.
- Never log authorization headers, cookies, proxy credentials, or response
  bodies by default.
- Add regression tests for security-sensitive changes.

## Commit messages

Use short, imperative commit subjects. Conventional Commits are welcome but
not required. Examples:

```text
feat: add cookie session support
fix: validate every redirect target
docs: expand Codex setup instructions
```

## License

By contributing, you agree that your contributions will be licensed under the
MIT License.
