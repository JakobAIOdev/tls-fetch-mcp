package fetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type policy struct {
	allowPrivate bool
	allowedHosts []string
	resolver     *net.Resolver
}

func newPolicy(cfg Config) policy {
	return policy{
		allowPrivate: cfg.AllowPrivate,
		allowedHosts: cfg.AllowedHosts,
		resolver:     net.DefaultResolver,
	}
}

func (p policy) validateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if err := p.validateParsedURL(ctx, parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (p policy) validateParsedURL(ctx context.Context, parsed *url.URL) error {
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	if parsed.User != nil {
		return fmt.Errorf("credentials in the URL are not allowed; use an Authorization header")
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("URL must contain a hostname")
	}
	if !p.hostAllowed(host) {
		return fmt.Errorf("host %q is not in MCP_TLS_FETCH_ALLOWED_HOSTS", host)
	}

	addresses, err := p.resolve(ctx, host)
	if err != nil {
		return err
	}
	for _, address := range addresses {
		if err := p.validateIP(address); err != nil {
			return fmt.Errorf("host %q: %w", host, err)
		}
	}
	return nil
}

func (p policy) validateProxyURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("proxy URL scheme must be http, https, socks5 or socks5h")
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("proxy URL must contain a hostname")
	}
	addresses, err := p.resolve(ctx, host)
	if err != nil {
		return err
	}
	for _, address := range addresses {
		if err := p.validateIP(address); err != nil {
			return fmt.Errorf("proxy host %q: %w", host, err)
		}
	}
	return nil
}

func (p policy) hostAllowed(host string) bool {
	if len(p.allowedHosts) == 0 {
		return true
	}
	for _, allowed := range p.allowedHosts {
		if host == allowed {
			return true
		}
		if strings.HasPrefix(allowed, "*.") {
			suffix := strings.TrimPrefix(allowed, "*.")
			if suffix != "" && strings.HasSuffix(host, "."+suffix) {
				return true
			}
		}
	}
	return false
}

func (p policy) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if address := net.ParseIP(host); address != nil {
		return []net.IP{address}, nil
	}
	addresses, err := p.resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("host %q did not resolve to an IP address", host)
	}
	return addresses, nil
}

func (p policy) validateIP(address net.IP) error {
	if address.IsUnspecified() || address.IsMulticast() {
		return fmt.Errorf("multicast and unspecified addresses are always blocked")
	}
	if address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() {
		if p.allowPrivate {
			return nil
		}
		return fmt.Errorf("private, loopback and link-local addresses are blocked")
	}
	if !address.IsGlobalUnicast() {
		return fmt.Errorf("address is not globally routable")
	}
	return nil
}

// dialContext validates the address immediately before connecting. This closes
// the usual DNS-rebinding gap between URL validation and the network dial.
func (p policy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address: %w", err)
	}
	addresses, err := p.resolve(ctx, host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	dialer := &net.Dialer{}
	for _, ip := range addresses {
		if err := p.validateIP(ip); err != nil {
			return nil, fmt.Errorf("refusing connection to %s: %w", ip, err)
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("connect to %q: %w", host, lastErr)
}
