# Security Policy

TLS Fetch MCP accepts network destinations and request metadata from an AI
client. Security reports involving SSRF, DNS rebinding, credential exposure,
request smuggling, unsafe redirects, unbounded resource use, or MCP schema
confusion are especially important.

## Supported versions

Security fixes are provided for the latest release on the `main` branch.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| `main` | Yes |
| Older releases | Best effort |

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability.

Use [GitHub private vulnerability reporting](https://github.com/JakobAIOdev/tls-fetch-mcp/security/advisories/new)
and include:

- A concise description and impact assessment
- Affected versions or commit
- Reproduction steps or a minimal proof of concept
- The expected safe behavior
- Any suggested mitigation

Please avoid accessing data that does not belong to you. Use a controlled test
environment whenever possible.

You should receive an initial response within seven days. Confirmed issues will
be coordinated privately until a fix and release are available.

## Security boundaries

The server provides several safeguards:

- Public destinations only by default
- Optional exact and wildcard host allowlists
- URL validation before DNS lookup
- IP validation before the request and again at connection time
- Policy checks for redirect destinations
- Bounded redirects, timeouts, response sizes, and cookie sessions
- Restricted transport-managed request headers
- Explicit opt-in for private targets and proxies

These safeguards do not make arbitrary web content trustworthy. Responses may
contain prompt injection, malicious markup, secrets, or misleading
instructions. MCP clients should treat fetched content as untrusted data.

Enabling `MCP_TLS_FETCH_ALLOW_PRIVATE` expands the network boundary to local,
private, and link-local destinations. Only enable it for trusted local
development environments.

Enabling `MCP_TLS_FETCH_ALLOW_PROXY` delegates the final connection and
potentially DNS resolution to a caller-selected proxy. Only use trusted
proxies and avoid exposing credentials in logs or issue reports.

## Out of scope

- Bypassing access controls, CAPTCHAs, or third-party anti-abuse systems
- Reports that require attacking systems without authorization
- Denial-of-service testing against public infrastructure
- Vulnerabilities in upstream dependencies without a demonstrated impact on
  this project
