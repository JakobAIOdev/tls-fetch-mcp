package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func TestDoFetchesAndTruncatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Request-Method", r.Method)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer server.Close()

	fetcher := New(Config{
		AllowPrivate:     true,
		MaxResponseBytes: 1024,
		DefaultTimeout:   5,
		MaxTimeout:       10,
		MaxSessions:      8,
	})
	output, err := fetcher.Do(context.Background(), Input{
		URL:              server.URL,
		MaxResponseBytes: 5,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if output.Status != http.StatusOK {
		t.Errorf("Status = %d, want %d", output.Status, http.StatusOK)
	}
	if output.Body != "hello" || !output.Truncated {
		t.Errorf("Body = %q, Truncated = %v; want %q, true", output.Body, output.Truncated, "hello")
	}
}

func TestDoRejectsForbiddenHeader(t *testing.T) {
	fetcher := New(Config{
		AllowPrivate:     true,
		MaxResponseBytes: 1024,
		DefaultTimeout:   5,
		MaxTimeout:       10,
		MaxSessions:      8,
	})
	_, err := fetcher.Do(context.Background(), Input{
		URL:     "http://127.0.0.1",
		Headers: map[string]string{"Host": "example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be set") {
		t.Fatalf("Do() error = %v, want forbidden-header error", err)
	}
}

func TestSetHeadersLetsSuppliedValuesOverrideDefaults(t *testing.T) {
	req, err := fhttp.NewRequest(fhttp.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := setHeaders(req, map[string]string{"user-agent": "custom-agent"}, nil, DefaultProfile); err != nil {
		t.Fatalf("setHeaders() error = %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != "custom-agent" {
		t.Errorf("User-Agent = %q, want custom-agent", got)
	}
}

func TestLimits(t *testing.T) {
	fetcher := New(Config{
		MaxResponseBytes: 100,
		DefaultTimeout:   5,
		MaxTimeout:       10,
		MaxSessions:      8,
	})
	if _, err := fetcher.timeout(11); err == nil {
		t.Error("timeout() succeeded above server maximum")
	}
	if _, err := fetcher.responseLimit(101); err == nil {
		t.Error("responseLimit() succeeded above server maximum")
	}
}

func TestCookieSessionPersistsAndClearsCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "active", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/check":
			cookie, err := r.Cookie("session")
			if err != nil {
				http.Error(w, "missing cookie", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(cookie.Value))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := New(Config{
		AllowPrivate:     true,
		MaxResponseBytes: 1024,
		DefaultTimeout:   5,
		MaxTimeout:       10,
		MaxSessions:      8,
	})
	if _, err := fetcher.Do(context.Background(), Input{
		URL:       server.URL + "/set",
		SessionID: "checkout-flow",
	}); err != nil {
		t.Fatalf("set cookie: %v", err)
	}
	output, err := fetcher.Do(context.Background(), Input{
		URL:       server.URL + "/check",
		SessionID: "checkout-flow",
	})
	if err != nil {
		t.Fatalf("check cookie: %v", err)
	}
	if output.Status != http.StatusOK || output.Body != "active" {
		t.Fatalf("cookie check returned status %d and body %q", output.Status, output.Body)
	}
	if output.CookiesStored == 0 {
		t.Error("CookiesStored = 0, want at least one cookie")
	}

	cleared, err := fetcher.ClearSession("checkout-flow")
	if err != nil {
		t.Fatalf("ClearSession() error = %v", err)
	}
	if !cleared {
		t.Error("ClearSession() = false, want true")
	}
	cleared, err = fetcher.ClearSession("checkout-flow")
	if err != nil {
		t.Fatalf("second ClearSession() error = %v", err)
	}
	if cleared {
		t.Error("second ClearSession() = true, want false")
	}
}

func TestSessionValidationAndLimit(t *testing.T) {
	fetcher := New(Config{MaxSessions: 1})
	if _, err := fetcher.session("invalid/session"); err == nil {
		t.Error("session() accepted an invalid identifier")
	}
	if _, err := fetcher.session("first"); err != nil {
		t.Fatalf("session(first) error = %v", err)
	}
	if _, err := fetcher.session("second"); err == nil {
		t.Error("session() succeeded above the configured session limit")
	}
}

func TestIntegrationTLSFetch(t *testing.T) {
	target := os.Getenv("TLS_FETCH_INTEGRATION_URL")
	if target == "" {
		t.Skip("set TLS_FETCH_INTEGRATION_URL to run the live TLS integration test")
	}

	fetcher := New(Config{
		MaxResponseBytes: 64 * 1024,
		DefaultTimeout:   15,
		MaxTimeout:       30,
		MaxSessions:      8,
	})
	output, err := fetcher.Do(context.Background(), Input{URL: target})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if output.Status < 200 || output.Status >= 400 {
		t.Fatalf("Status = %d, want a successful response", output.Status)
	}
	if output.Profile != DefaultProfile {
		t.Errorf("Profile = %q, want %q", output.Profile, DefaultProfile)
	}
}
