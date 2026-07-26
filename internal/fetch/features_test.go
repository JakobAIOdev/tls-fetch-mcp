package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDoRedactsSensitiveResponseHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "top-secret", Path: "/"})
		w.Header().Set("Authorization", "Bearer secret")
		w.Header().Set("X-Public", "visible")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	fetcher := New(Config{AllowPrivate: true})
	output, err := fetcher.Do(context.Background(), Input{
		URL:       server.URL,
		SessionID: "redaction-test",
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if output.Headers["Authorization"] != nil || output.Headers["Set-Cookie"] != nil {
		t.Fatalf("sensitive headers leaked: %#v", output.Headers)
	}
	if output.Headers["X-Public"][0] != "visible" {
		t.Fatalf("public header missing: %#v", output.Headers)
	}
	if !slices.Contains(output.RedactedHeaders, "Authorization") || !slices.Contains(output.RedactedHeaders, "Set-Cookie") {
		t.Fatalf("RedactedHeaders = %v", output.RedactedHeaders)
	}
	if output.CookiesStored != 1 || !slices.Contains(output.CookieNames, "session") {
		t.Fatalf("cookie metadata = %d %v", output.CookiesStored, output.CookieNames)
	}
	for _, value := range output.CookieNames {
		if strings.Contains(value, "top-secret") {
			t.Fatal("cookie value leaked through cookie metadata")
		}
	}
}

func TestReadOnlyAndMutatingMethodSeparation(t *testing.T) {
	fetcher := New(Config{})
	if _, err := fetcher.DoGet(context.Background(), GetInput{
		URL:    "https://example.com",
		Method: http.MethodPost,
	}); err == nil || !strings.Contains(err.Error(), "tls_get only supports") {
		t.Fatalf("DoGet(POST) error = %v", err)
	}
	if _, err := fetcher.DoRequest(context.Background(), RequestInput{
		URL:    "https://example.com",
		Method: http.MethodGet,
	}); err == nil || !strings.Contains(err.Error(), "use tls_get") {
		t.Fatalf("DoRequest(GET) error = %v", err)
	}
	if _, err := fetcher.DoRequest(context.Background(), RequestInput{
		URL: "https://example.com",
	}); err == nil || !strings.Contains(err.Error(), "method is required") {
		t.Fatalf("DoRequest(empty method) error = %v", err)
	}
}

func TestStoredResponseCanBeReadAndSearched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Alpha catalog item\nBeta catalog item\n"))
	}))
	defer server.Close()

	fetcher := New(Config{
		AllowPrivate: true,
		MaxReadBytes: 8,
	})
	includeBody := false
	output, err := fetcher.DoGet(context.Background(), GetInput{
		URL:           server.URL,
		IncludeBody:   &includeBody,
		StoreResponse: true,
	})
	if err != nil {
		t.Fatalf("DoGet() error = %v", err)
	}
	if output.Body != "" || output.ResponseID == "" {
		t.Fatalf("body = %q, response_id = %q", output.Body, output.ResponseID)
	}

	first, err := fetcher.ReadResponse(ResponseReadInput{
		ResponseID: output.ResponseID,
		MaxBytes:   5,
	})
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if first.Body != "Alpha" || first.NextOffset != 5 || first.EOF {
		t.Fatalf("first read = %+v", first)
	}
	second, err := fetcher.ReadResponse(ResponseReadInput{
		ResponseID: output.ResponseID,
		Offset:     first.NextOffset,
		MaxBytes:   8,
	})
	if err != nil {
		t.Fatalf("second ReadResponse() error = %v", err)
	}
	if second.Body != " catalog" {
		t.Fatalf("second body = %q", second.Body)
	}

	search, err := fetcher.SearchResponse(ResponseSearchInput{
		ResponseID:   output.ResponseID,
		Query:        "CATALOG",
		MaxMatches:   1,
		ContextBytes: 6,
	})
	if err != nil {
		t.Fatalf("SearchResponse() error = %v", err)
	}
	if search.MatchCount != 1 || !search.Limited || !strings.Contains(search.Matches[0].Context, "catalog") {
		t.Fatalf("search = %+v", search)
	}
}

func TestResponseAndSessionTTL(t *testing.T) {
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	fetcher := New(Config{
		SessionTTL:   1,
		ResponseTTL:  1,
		MaxResponses: 2,
	})
	fetcher.now = func() time.Time { return now }

	if _, err := fetcher.session("ttl-session"); err != nil {
		t.Fatalf("session() error = %v", err)
	}
	responseID, err := fetcher.storeResponse([]byte("temporary"), storedResponseMetadata{
		BodyEncoding: "utf-8",
	})
	if err != nil {
		t.Fatalf("storeResponse() error = %v", err)
	}
	now = now.Add(2 * time.Second)

	info, err := fetcher.SessionInfo("ttl-session")
	if err != nil {
		t.Fatalf("SessionInfo() error = %v", err)
	}
	if info.Exists {
		t.Fatalf("expired session still exists: %+v", info)
	}
	if _, err := fetcher.ReadResponse(ResponseReadInput{ResponseID: responseID}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("ReadResponse(expired) error = %v", err)
	}
}

func TestRedirectHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("done"))
	}))
	defer server.Close()

	fetcher := New(Config{AllowPrivate: true})
	output, err := fetcher.DoGet(context.Background(), GetInput{URL: server.URL + "/start"})
	if err != nil {
		t.Fatalf("DoGet() error = %v", err)
	}
	if len(output.RedirectHistory) != 1 || output.RedirectHistory[0] != server.URL+"/final" {
		t.Fatalf("RedirectHistory = %v", output.RedirectHistory)
	}
	if output.FinalURL != server.URL+"/final" {
		t.Fatalf("FinalURL = %q", output.FinalURL)
	}
}
