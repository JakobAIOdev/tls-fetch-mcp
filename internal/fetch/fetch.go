package fetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const DefaultProfile = "chrome_146"

var allowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

var forbiddenRequestHeaders = map[string]struct{}{
	"connection":        {},
	"content-length":    {},
	"host":              {},
	"keep-alive":        {},
	"proxy-connection":  {},
	"te":                {},
	"trailer":           {},
	"transfer-encoding": {},
	"upgrade":           {},
}

type Input struct {
	URL              string            `json:"url" jsonschema:"Absolute http or https URL to request"`
	Method           string            `json:"method,omitempty" jsonschema:"HTTP method: GET, HEAD, POST, PUT, PATCH, DELETE or OPTIONS; defaults to GET"`
	Headers          map[string]string `json:"headers,omitempty" jsonschema:"Request headers as name-value pairs"`
	HeaderOrder      []string          `json:"header_order,omitempty" jsonschema:"Optional lower-case HTTP header order for accurate fingerprinting"`
	Body             string            `json:"body,omitempty" jsonschema:"Raw request body"`
	Profile          string            `json:"profile,omitempty" jsonschema:"TLS browser profile; defaults to chrome_146. Call tls_profiles for all names"`
	FollowRedirects  *bool             `json:"follow_redirects,omitempty" jsonschema:"Follow redirects; defaults to true"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty" jsonschema:"Whole-request timeout in seconds"`
	MaxResponseBytes int64             `json:"max_response_bytes,omitempty" jsonschema:"Maximum response bytes to return; capped by the server"`
	ProxyURL         string            `json:"proxy_url,omitempty" jsonschema:"Optional HTTP or SOCKS proxy; disabled unless the server permits proxies"`
	SessionID        string            `json:"session_id,omitempty" jsonschema:"Optional cookie-session identifier using letters, digits, dots, underscores or hyphens"`
	IncludeBody      *bool             `json:"include_body,omitempty" jsonschema:"Include the response body inline; defaults to true"`
	StoreResponse    bool              `json:"store_response,omitempty" jsonschema:"Store the bounded response body temporarily and return a response_id for extract/search/read tools"`
}

type Output struct {
	Status          int                 `json:"status"`
	StatusText      string              `json:"status_text"`
	FinalURL        string              `json:"final_url"`
	Headers         map[string][]string `json:"headers"`
	Body            string              `json:"body"`
	BodyEncoding    string              `json:"body_encoding"`
	ContentType     string              `json:"content_type,omitempty"`
	Truncated       bool                `json:"truncated"`
	ElapsedMillis   int64               `json:"elapsed_ms"`
	Profile         string              `json:"profile"`
	SessionID       string              `json:"session_id,omitempty"`
	CookiesStored   int                 `json:"cookies_stored,omitempty"`
	CookieNames     []string            `json:"cookie_names,omitempty"`
	RedactedHeaders []string            `json:"redacted_headers,omitempty"`
	RedirectHistory []string            `json:"redirect_history,omitempty"`
	HTTPVersion     string              `json:"http_version,omitempty"`
	ContentLength   int64               `json:"content_length"`
	BytesReturned   int64               `json:"bytes_returned"`
	ResponseID      string              `json:"response_id,omitempty"`
}

type ProfilesInput struct{}

type ProfilesOutput struct {
	Default  string   `json:"default"`
	Profiles []string `json:"profiles"`
}

type SessionClearInput struct {
	SessionID string `json:"session_id" jsonschema:"Cookie-session identifier to clear"`
}

type SessionClearOutput struct {
	SessionID string `json:"session_id"`
	Cleared   bool   `json:"cleared"`
}

type Fetcher struct {
	config    Config
	policy    policy
	sessions  map[string]*sessionState
	responses map[string]*storedResponse
	now       func() time.Time
	mu        sync.Mutex
}

func New(config Config) *Fetcher {
	config = config.withDefaults()
	return &Fetcher{
		config:    config,
		policy:    newPolicy(config),
		sessions:  make(map[string]*sessionState),
		responses: make(map[string]*storedResponse),
		now:       time.Now,
	}
}

func ProfileNames() []string {
	names := make([]string, 0, len(profiles.MappedTLSClients))
	for name := range profiles.MappedTLSClients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (f *Fetcher) Do(ctx context.Context, input Input) (Output, error) {
	if strings.TrimSpace(input.URL) == "" {
		return Output{}, fmt.Errorf("url is required")
	}
	parsedURL, err := f.policy.validateURL(ctx, input.URL)
	if err != nil {
		return Output{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	if _, ok := allowedMethods[method]; !ok {
		return Output{}, fmt.Errorf("unsupported method %q", method)
	}

	profileName := strings.ToLower(strings.TrimSpace(input.Profile))
	if profileName == "" {
		profileName = DefaultProfile
	}
	clientProfile, ok := profiles.MappedTLSClients[profileName]
	if !ok {
		return Output{}, fmt.Errorf("unknown TLS profile %q; call tls_profiles for supported names", profileName)
	}

	timeout, err := f.timeout(input.TimeoutSeconds)
	if err != nil {
		return Output{}, err
	}
	responseLimit, err := f.responseLimit(input.MaxResponseBytes)
	if err != nil {
		return Output{}, err
	}
	followRedirects := input.FollowRedirects == nil || *input.FollowRedirects
	includeBody := input.IncludeBody == nil || *input.IncludeBody

	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(timeout),
		tlsclient.WithClientProfile(clientProfile),
		tlsclient.WithCatchPanics(),
		tlsclient.WithDisableHttp3(),
	}
	var sessionJar tlsclient.CookieJar
	if input.SessionID != "" {
		sessionJar, err = f.session(input.SessionID)
		if err != nil {
			return Output{}, err
		}
		options = append(options, tlsclient.WithCookieJar(sessionJar))
	}
	var redirectHistory []string
	if !followRedirects {
		options = append(options, tlsclient.WithNotFollowRedirects())
	} else {
		options = append(options, tlsclient.WithCustomRedirectFunc(func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if err := f.policy.validateParsedURL(req.Context(), req.URL); err != nil {
				return err
			}
			redirectHistory = append(redirectHistory, req.URL.String())
			return nil
		}))
	}
	if input.ProxyURL != "" {
		if !f.config.AllowProxy {
			return Output{}, fmt.Errorf("proxy_url is disabled; set MCP_TLS_FETCH_ALLOW_PROXY=true to enable it")
		}
		if err := f.policy.validateProxyURL(ctx, input.ProxyURL); err != nil {
			return Output{}, fmt.Errorf("invalid proxy_url: %w", err)
		}
		options = append(options, tlsclient.WithProxyUrl(input.ProxyURL))
	} else {
		options = append(options, tlsclient.WithDialContext(f.policy.dialContext))
	}

	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return Output{}, fmt.Errorf("create TLS client: %w", err)
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewBufferString(input.Body))
	if err != nil {
		return Output{}, fmt.Errorf("create request: %w", err)
	}
	if err := setHeaders(req, input.Headers, input.HeaderOrder, profileName); err != nil {
		return Output{}, err
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Output{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		return Output{}, fmt.Errorf("read response: %w", err)
	}
	truncated := int64(len(body)) > responseLimit
	if truncated {
		body = body[:responseLimit]
	}

	bodyText, encoding := encodeBody(body)
	if !includeBody {
		bodyText = ""
	}
	cookiesStored, cookieNames := 0, []string(nil)
	if sessionJar != nil {
		cookiesStored, cookieNames = cookieSummary(sessionJar)
		f.touchSession(input.SessionID)
	}
	headers, redactedHeaders := sanitizeResponseHeaders(resp.Header)
	finalURL := parsedURL.String()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	responseID := ""
	if input.StoreResponse {
		responseID, err = f.storeResponse(body, storedResponseMetadata{
			BodyEncoding: encoding,
			ContentType:  resp.Header.Get("Content-Type"),
			FinalURL:     finalURL,
			Truncated:    truncated,
		})
		if err != nil {
			return Output{}, err
		}
	}
	return Output{
		Status:          resp.StatusCode,
		StatusText:      http.StatusText(resp.StatusCode),
		FinalURL:        finalURL,
		Headers:         headers,
		Body:            bodyText,
		BodyEncoding:    encoding,
		ContentType:     resp.Header.Get("Content-Type"),
		Truncated:       truncated,
		ElapsedMillis:   time.Since(started).Milliseconds(),
		Profile:         profileName,
		SessionID:       input.SessionID,
		CookiesStored:   cookiesStored,
		CookieNames:     cookieNames,
		RedactedHeaders: redactedHeaders,
		RedirectHistory: redirectHistory,
		HTTPVersion:     resp.Proto,
		ContentLength:   resp.ContentLength,
		BytesReturned:   int64(len(body)),
		ResponseID:      responseID,
	}, nil
}

func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if len(sessionID) > 128 {
		return fmt.Errorf("session_id must not exceed 128 characters")
	}
	for _, char := range sessionID {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' ||
			char == '_' ||
			char == '-' {
			continue
		}
		return fmt.Errorf("session_id may only contain letters, digits, dots, underscores and hyphens")
	}
	return nil
}

func (f *Fetcher) timeout(requested int) (int, error) {
	if requested == 0 {
		return f.config.DefaultTimeout, nil
	}
	if requested < 0 || requested > f.config.MaxTimeout {
		return 0, fmt.Errorf("timeout_seconds must be between 1 and %d", f.config.MaxTimeout)
	}
	return requested, nil
}

func (f *Fetcher) responseLimit(requested int64) (int64, error) {
	if requested == 0 {
		return f.config.MaxResponseBytes, nil
	}
	if requested < 0 || requested > f.config.MaxResponseBytes {
		return 0, fmt.Errorf("max_response_bytes must be between 1 and %d", f.config.MaxResponseBytes)
	}
	return requested, nil
}

func setHeaders(req *http.Request, supplied map[string]string, order []string, profile string) error {
	for name, value := range defaultHeaders(profile) {
		req.Header.Set(name, value)
	}
	for name, value := range supplied {
		canonicalName := strings.ToLower(strings.TrimSpace(name))
		if canonicalName == "" {
			return fmt.Errorf("header names cannot be empty")
		}
		if _, forbidden := forbiddenRequestHeaders[canonicalName]; forbidden {
			return fmt.Errorf("request header %q is managed by the HTTP client and cannot be set", name)
		}
		if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("request headers cannot contain newlines")
		}
		req.Header.Set(name, value)
	}

	if len(order) == 0 {
		order = []string{"accept", "accept-language", "cache-control", "user-agent"}
	}
	normalizedOrder := make([]string, 0, len(order))
	for _, name := range order {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			normalizedOrder = append(normalizedOrder, name)
		}
	}
	req.Header[http.HeaderOrderKey] = normalizedOrder
	return nil
}

func defaultHeaders(profile string) map[string]string {
	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	if strings.HasPrefix(profile, "firefox_") {
		version := profileVersion(profile, "148")
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:" + version + ".0) Gecko/20100101 Firefox/" + version + ".0"
	} else if strings.HasPrefix(profile, "safari_") {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15"
	} else if strings.HasPrefix(profile, "chrome_") || strings.HasPrefix(profile, "brave_") {
		version := profileVersion(profile, "146")
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + ".0.0.0 Safari/537.36"
	}
	return map[string]string{
		"Accept":          "*/*",
		"Accept-Language": "en-US,en;q=0.9",
		"Cache-Control":   "no-cache",
		"User-Agent":      userAgent,
	}
}

func profileVersion(profile, fallback string) string {
	parts := strings.Split(profile, "_")
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			return part
		}
	}
	return fallback
}

func encodeBody(body []byte) (string, string) {
	if utf8.Valid(body) {
		return string(body), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}
