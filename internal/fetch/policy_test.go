package fetch

import (
	"context"
	"testing"
)

func TestPolicyBlocksPrivateTargetsByDefault(t *testing.T) {
	p := newPolicy(Config{})
	blocked := []string{
		"http://127.0.0.1:8080",
		"http://10.0.0.1",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]",
	}
	for _, target := range blocked {
		if _, err := p.validateURL(context.Background(), target); err == nil {
			t.Errorf("validateURL(%q) succeeded, want private-address error", target)
		}
	}
}

func TestPolicyAllowsPrivateTargetsWhenConfigured(t *testing.T) {
	p := newPolicy(Config{AllowPrivate: true})
	if _, err := p.validateURL(context.Background(), "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("validateURL() error = %v", err)
	}
	for _, target := range []string{"http://0.0.0.0", "http://224.0.0.1"} {
		if _, err := p.validateURL(context.Background(), target); err == nil {
			t.Errorf("validateURL(%q) succeeded, want unroutable-address error", target)
		}
	}
}

func TestPolicyEnforcesAllowedHosts(t *testing.T) {
	p := newPolicy(Config{
		AllowPrivate: true,
		AllowedHosts: []string{"127.0.0.1", "*.example.org"},
	})
	if _, err := p.validateURL(context.Background(), "http://127.0.0.1"); err != nil {
		t.Fatalf("validateURL() error = %v", err)
	}
	if _, err := p.validateURL(context.Background(), "http://127.0.0.2"); err == nil {
		t.Fatal("validateURL() succeeded for a host outside the allowlist")
	}

	hostTests := []struct {
		host    string
		allowed bool
	}{
		{"api.example.org", true},
		{"deep.api.example.org", true},
		{"example.org", false},
		{"notexample.org", false},
	}
	for _, test := range hostTests {
		if got := p.hostAllowed(test.host); got != test.allowed {
			t.Errorf("hostAllowed(%q) = %v, want %v", test.host, got, test.allowed)
		}
	}
}

func TestPolicyRejectsUnsafeURLForms(t *testing.T) {
	p := newPolicy(Config{AllowPrivate: true})
	targets := []string{
		"file:///etc/passwd",
		"https://user:password@example.com",
		"https:///missing-host",
	}
	for _, target := range targets {
		if _, err := p.validateURL(context.Background(), target); err == nil {
			t.Errorf("validateURL(%q) succeeded, want error", target)
		}
	}
}

func TestPolicyAcceptsAuthenticatedSOCKSProxy(t *testing.T) {
	p := newPolicy(Config{AllowPrivate: true})
	if err := p.validateProxyURL(context.Background(), "socks5://user:password@127.0.0.1:1080"); err != nil {
		t.Fatalf("validateProxyURL() error = %v", err)
	}
}
