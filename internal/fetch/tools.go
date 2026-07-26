package fetch

import (
	"context"
	"fmt"
	"strings"

	http "github.com/bogdanfinn/fhttp"
)

type GetInput struct {
	URL              string            `json:"url" jsonschema:"Absolute http or https URL to request"`
	Method           string            `json:"method,omitempty" jsonschema:"Read-only HTTP method: GET or HEAD; defaults to GET"`
	Headers          map[string]string `json:"headers,omitempty" jsonschema:"Request headers as name-value pairs"`
	HeaderOrder      []string          `json:"header_order,omitempty" jsonschema:"Optional lower-case HTTP header order for accurate fingerprinting"`
	Profile          string            `json:"profile,omitempty" jsonschema:"TLS browser profile; defaults to chrome_146. Call tls_profiles for all names"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty" jsonschema:"Follow redirects; defaults to true"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty" jsonschema:"Whole-request timeout in seconds"`
	MaxResponseBytes int64             `json:"max_response_bytes,omitempty" jsonschema:"Maximum response bytes to return or store; capped by the server"`
	ProxyURL         string            `json:"proxy_url,omitempty" jsonschema:"Optional HTTP or SOCKS proxy; disabled unless the server permits proxies"`
	SessionID        string            `json:"session_id,omitempty" jsonschema:"Optional cookie-session identifier using letters, digits, dots, underscores or hyphens"`
	IncludeBody      *bool             `json:"include_body,omitempty" jsonschema:"Include the response body inline; defaults to true"`
	StoreResponse    bool              `json:"store_response,omitempty" jsonschema:"Store the bounded response body temporarily and return a response_id for read/search tools"`
}

type RequestInput struct {
	URL              string            `json:"url" jsonschema:"Absolute http or https URL to request"`
	Method           string            `json:"method" jsonschema:"Mutating HTTP method: POST, PUT, PATCH, DELETE or OPTIONS"`
	Headers          map[string]string `json:"headers,omitempty" jsonschema:"Request headers as name-value pairs"`
	HeaderOrder      []string          `json:"header_order,omitempty" jsonschema:"Optional lower-case HTTP header order for accurate fingerprinting"`
	Body             string            `json:"body,omitempty" jsonschema:"Raw request body"`
	Profile          string            `json:"profile,omitempty" jsonschema:"TLS browser profile; defaults to chrome_146. Call tls_profiles for all names"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty" jsonschema:"Follow redirects; defaults to true"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty" jsonschema:"Whole-request timeout in seconds"`
	MaxResponseBytes int64             `json:"max_response_bytes,omitempty" jsonschema:"Maximum response bytes to return or store; capped by the server"`
	ProxyURL         string            `json:"proxy_url,omitempty" jsonschema:"Optional HTTP or SOCKS proxy; disabled unless the server permits proxies"`
	SessionID        string            `json:"session_id,omitempty" jsonschema:"Optional cookie-session identifier using letters, digits, dots, underscores or hyphens"`
	IncludeBody      *bool             `json:"include_body,omitempty" jsonschema:"Include the response body inline; defaults to true"`
	StoreResponse    bool              `json:"store_response,omitempty" jsonschema:"Store the bounded response body temporarily and return a response_id for read/search tools"`
}

type SessionWarmupInput struct {
	URL              string            `json:"url" jsonschema:"Absolute http or https URL used to initialize the cookie session"`
	SessionID        string            `json:"session_id" jsonschema:"Cookie-session identifier to create or refresh"`
	Headers          map[string]string `json:"headers,omitempty" jsonschema:"Request headers as name-value pairs"`
	HeaderOrder      []string          `json:"header_order,omitempty" jsonschema:"Optional lower-case HTTP header order"`
	Profile          string            `json:"profile,omitempty" jsonschema:"TLS browser profile; defaults to chrome_146"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty" jsonschema:"Follow redirects; defaults to true"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty" jsonschema:"Whole-request timeout in seconds"`
	MaxResponseBytes int64             `json:"max_response_bytes,omitempty" jsonschema:"Maximum bytes to consume; capped by the server"`
	ProxyURL         string            `json:"proxy_url,omitempty" jsonschema:"Optional proxy; disabled unless the server permits proxies"`
}

func (f *Fetcher) DoGet(ctx context.Context, input GetInput) (Output, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return Output{}, fmt.Errorf("tls_get only supports GET and HEAD; use tls_request for %s", method)
	}
	return f.Do(ctx, Input{
		URL:              input.URL,
		Method:           method,
		Headers:          input.Headers,
		HeaderOrder:      input.HeaderOrder,
		Profile:          input.Profile,
		FollowRedirects:  input.FollowRedirects,
		TimeoutSeconds:   input.TimeoutSeconds,
		MaxResponseBytes: input.MaxResponseBytes,
		ProxyURL:         input.ProxyURL,
		SessionID:        input.SessionID,
		IncludeBody:      input.IncludeBody,
		StoreResponse:    input.StoreResponse,
	})
}

func (f *Fetcher) DoRequest(ctx context.Context, input RequestInput) (Output, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		return Output{}, fmt.Errorf("method is required; use tls_get for GET or HEAD")
	}
	if method == http.MethodGet || method == http.MethodHead {
		return Output{}, fmt.Errorf("use tls_get for read-only %s requests", method)
	}
	return f.Do(ctx, Input{
		URL:              input.URL,
		Method:           method,
		Headers:          input.Headers,
		HeaderOrder:      input.HeaderOrder,
		Body:             input.Body,
		Profile:          input.Profile,
		FollowRedirects:  input.FollowRedirects,
		TimeoutSeconds:   input.TimeoutSeconds,
		MaxResponseBytes: input.MaxResponseBytes,
		ProxyURL:         input.ProxyURL,
		SessionID:        input.SessionID,
		IncludeBody:      input.IncludeBody,
		StoreResponse:    input.StoreResponse,
	})
}

func (f *Fetcher) WarmupSession(ctx context.Context, input SessionWarmupInput) (Output, error) {
	if err := validateSessionID(input.SessionID); err != nil {
		return Output{}, err
	}
	includeBody := false
	return f.DoGet(ctx, GetInput{
		URL:              input.URL,
		Headers:          input.Headers,
		HeaderOrder:      input.HeaderOrder,
		Profile:          input.Profile,
		FollowRedirects:  input.FollowRedirects,
		TimeoutSeconds:   input.TimeoutSeconds,
		MaxResponseBytes: input.MaxResponseBytes,
		ProxyURL:         input.ProxyURL,
		SessionID:        input.SessionID,
		IncludeBody:      &includeBody,
	})
}
