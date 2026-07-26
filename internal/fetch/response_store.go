package fetch

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultResponseReadBytes = 64 * 1024
	defaultSearchMatches     = 20
	maxSearchMatches         = 50
	defaultSearchContext     = 160
	maxSearchContext         = 1024
	maxSearchQueryBytes      = 1024
)

var sensitiveResponseHeaders = map[string]struct{}{
	"authentication-info":       {},
	"authorization":             {},
	"proxy-authenticate":        {},
	"proxy-authentication-info": {},
	"proxy-authorization":       {},
	"set-cookie":                {},
	"set-cookie2":               {},
}

type storedResponseMetadata struct {
	BodyEncoding string
	ContentType  string
	FinalURL     string
	Truncated    bool
}

type storedResponse struct {
	body         []byte
	bodyEncoding string
	contentType  string
	finalURL     string
	truncated    bool
	createdAt    time.Time
	expiresAt    time.Time
}

type ResponseReadInput struct {
	ResponseID string `json:"response_id" jsonschema:"Temporary response identifier returned by tls_get, tls_request or tls_fetch"`
	Offset     int64  `json:"offset,omitempty" jsonschema:"Zero-based byte offset; defaults to 0"`
	MaxBytes   int64  `json:"max_bytes,omitempty" jsonschema:"Maximum raw bytes to read; defaults to 65536 and is capped by the server"`
}

type ResponseReadOutput struct {
	ResponseID   string `json:"response_id"`
	Offset       int64  `json:"offset"`
	NextOffset   int64  `json:"next_offset"`
	TotalBytes   int64  `json:"total_bytes"`
	Body         string `json:"body"`
	BodyEncoding string `json:"body_encoding"`
	EOF          bool   `json:"eof"`
	ContentType  string `json:"content_type,omitempty"`
	FinalURL     string `json:"final_url,omitempty"`
	Truncated    bool   `json:"truncated"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at"`
}

type ResponseSearchInput struct {
	ResponseID    string `json:"response_id" jsonschema:"Temporary response identifier returned by tls_get, tls_request or tls_fetch"`
	Query         string `json:"query" jsonschema:"Literal UTF-8 text to find in the stored response"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"Use case-sensitive matching; defaults to false"`
	MaxMatches    int    `json:"max_matches,omitempty" jsonschema:"Maximum matches to return; defaults to 20 and is capped at 50"`
	ContextBytes  int    `json:"context_bytes,omitempty" jsonschema:"Context bytes around each match; defaults to 160 and is capped at 1024"`
}

type ResponseSearchMatch struct {
	Offset  int64  `json:"offset"`
	Context string `json:"context"`
}

type ResponseSearchOutput struct {
	ResponseID string                `json:"response_id"`
	Query      string                `json:"query"`
	Matches    []ResponseSearchMatch `json:"matches"`
	MatchCount int                   `json:"match_count"`
	Limited    bool                  `json:"limited"`
	TotalBytes int64                 `json:"total_bytes"`
}

func sanitizeResponseHeaders(headers map[string][]string) (map[string][]string, []string) {
	safe := make(map[string][]string, len(headers))
	var redacted []string
	for name, values := range headers {
		if _, sensitive := sensitiveResponseHeaders[strings.ToLower(name)]; sensitive {
			redacted = append(redacted, name)
			continue
		}
		safe[name] = append([]string(nil), values...)
	}
	sortStringsCaseInsensitive(redacted)
	return safe, redacted
}

func sortStringsCaseInsensitive(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && strings.ToLower(values[j]) < strings.ToLower(values[j-1]); j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (f *Fetcher) storeResponse(body []byte, metadata storedResponseMetadata) (string, error) {
	id, err := newResponseID()
	if err != nil {
		return "", fmt.Errorf("create response handle: %w", err)
	}
	now := f.now()
	record := &storedResponse{
		body:         append([]byte(nil), body...),
		bodyEncoding: metadata.BodyEncoding,
		contentType:  metadata.ContentType,
		finalURL:     metadata.FinalURL,
		truncated:    metadata.Truncated,
		createdAt:    now,
		expiresAt:    now.Add(time.Duration(f.config.ResponseTTL) * time.Second),
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeExpiredResponsesLocked(now)
	for len(f.responses) >= f.config.MaxResponses {
		f.evictOldestResponseLocked()
	}
	f.responses[id] = record
	return id, nil
}

func (f *Fetcher) ReadResponse(input ResponseReadInput) (ResponseReadOutput, error) {
	record, err := f.response(input.ResponseID)
	if err != nil {
		return ResponseReadOutput{}, err
	}
	if input.Offset < 0 || input.Offset > int64(len(record.body)) {
		return ResponseReadOutput{}, fmt.Errorf("offset must be between 0 and %d", len(record.body))
	}
	maxBytes := input.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultResponseReadBytes
		if maxBytes > f.config.MaxReadBytes {
			maxBytes = f.config.MaxReadBytes
		}
	}
	if maxBytes < 0 || maxBytes > f.config.MaxReadBytes {
		return ResponseReadOutput{}, fmt.Errorf("max_bytes must be between 1 and %d", f.config.MaxReadBytes)
	}
	end := input.Offset + maxBytes
	if end > int64(len(record.body)) {
		end = int64(len(record.body))
	}
	body, encoding := encodeBody(record.body[input.Offset:end])
	return ResponseReadOutput{
		ResponseID:   input.ResponseID,
		Offset:       input.Offset,
		NextOffset:   end,
		TotalBytes:   int64(len(record.body)),
		Body:         body,
		BodyEncoding: encoding,
		EOF:          end == int64(len(record.body)),
		ContentType:  record.contentType,
		FinalURL:     record.finalURL,
		Truncated:    record.truncated,
		CreatedAt:    record.createdAt.UTC().Format(time.RFC3339),
		ExpiresAt:    record.expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (f *Fetcher) SearchResponse(input ResponseSearchInput) (ResponseSearchOutput, error) {
	record, err := f.response(input.ResponseID)
	if err != nil {
		return ResponseSearchOutput{}, err
	}
	if !utf8.Valid(record.body) {
		return ResponseSearchOutput{}, fmt.Errorf("response_search requires a UTF-8 response; use tls_response_read for base64 chunks")
	}
	query := []byte(input.Query)
	if len(query) == 0 {
		return ResponseSearchOutput{}, fmt.Errorf("query is required")
	}
	if !utf8.Valid(query) || len(query) > maxSearchQueryBytes {
		return ResponseSearchOutput{}, fmt.Errorf("query must be valid UTF-8 and at most %d bytes", maxSearchQueryBytes)
	}
	maxMatches := input.MaxMatches
	if maxMatches == 0 {
		maxMatches = defaultSearchMatches
	}
	if maxMatches < 1 || maxMatches > maxSearchMatches {
		return ResponseSearchOutput{}, fmt.Errorf("max_matches must be between 1 and %d", maxSearchMatches)
	}
	contextBytes := input.ContextBytes
	if contextBytes == 0 {
		contextBytes = defaultSearchContext
	}
	if contextBytes < 0 || contextBytes > maxSearchContext {
		return ResponseSearchOutput{}, fmt.Errorf("context_bytes must be between 0 and %d", maxSearchContext)
	}

	haystack := record.body
	needle := query
	if !input.CaseSensitive {
		haystack = asciiLowerCopy(haystack)
		needle = asciiLowerCopy(needle)
	}
	matches := make([]ResponseSearchMatch, 0, maxMatches)
	searchFrom := 0
	limited := false
	for searchFrom <= len(haystack)-len(needle) {
		relative := bytes.Index(haystack[searchFrom:], needle)
		if relative < 0 {
			break
		}
		offset := searchFrom + relative
		if len(matches) == maxMatches {
			limited = true
			break
		}
		start := offset - contextBytes
		if start < 0 {
			start = 0
		}
		end := offset + len(query) + contextBytes
		if end > len(record.body) {
			end = len(record.body)
		}
		matches = append(matches, ResponseSearchMatch{
			Offset:  int64(offset),
			Context: validUTF8Window(record.body, start, end),
		})
		searchFrom = offset + len(needle)
	}
	return ResponseSearchOutput{
		ResponseID: input.ResponseID,
		Query:      input.Query,
		Matches:    matches,
		MatchCount: len(matches),
		Limited:    limited,
		TotalBytes: int64(len(record.body)),
	}, nil
}

func (f *Fetcher) response(id string) (*storedResponse, error) {
	if !strings.HasPrefix(id, "resp_") || len(id) != 37 {
		return nil, fmt.Errorf("invalid response_id")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeExpiredResponsesLocked(f.now())
	record, exists := f.responses[id]
	if !exists {
		return nil, fmt.Errorf("response_id was not found or has expired")
	}
	return record, nil
}

func (f *Fetcher) purgeExpiredResponsesLocked(now time.Time) {
	for id, record := range f.responses {
		if !record.expiresAt.After(now) {
			delete(f.responses, id)
		}
	}
}

func (f *Fetcher) evictOldestResponseLocked() {
	var oldestID string
	var oldest time.Time
	for id, record := range f.responses {
		if oldestID == "" || record.createdAt.Before(oldest) {
			oldestID = id
			oldest = record.createdAt
		}
	}
	if oldestID != "" {
		delete(f.responses, oldestID)
	}
}

func newResponseID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "resp_" + hex.EncodeToString(value), nil
}

func asciiLowerCopy(value []byte) []byte {
	result := append([]byte(nil), value...)
	for i, char := range result {
		if char >= 'A' && char <= 'Z' {
			result[i] = char + ('a' - 'A')
		}
	}
	return result
}

func validUTF8Window(body []byte, start, end int) string {
	for start > 0 && !utf8.RuneStart(body[start]) {
		start--
	}
	for end < len(body) && !utf8.Valid(body[start:end]) {
		end++
	}
	return string(body[start:end])
}
