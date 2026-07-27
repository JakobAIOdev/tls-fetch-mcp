package fetch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/cascadia"
	"github.com/theory/jsonpath"
	"golang.org/x/net/html"
)

const (
	defaultExtractResults     = 20
	maxExtractResults         = 100
	maxExtractQueries         = 16
	maxExtractNameBytes       = 128
	maxExtractSelectorBytes   = 2 * 1024
	defaultExtractOutputBytes = 64 * 1024
)

var htmlURLAttributes = map[string]struct{}{
	"action":     {},
	"cite":       {},
	"data":       {},
	"formaction": {},
	"href":       {},
	"poster":     {},
	"src":        {},
}

type ResponseExtractInput struct {
	ResponseID     string         `json:"response_id" jsonschema:"Temporary response identifier returned by tls_get, tls_request or tls_fetch"`
	Format         string         `json:"format,omitempty" jsonschema:"Extraction format: auto, html or json; defaults to auto using Content-Type and body detection"`
	Queries        []ExtractQuery `json:"queries" jsonschema:"One to sixteen named CSS selector or RFC 9535 JSONPath queries"`
	MaxResults     int            `json:"max_results,omitempty" jsonschema:"Maximum values returned per query; defaults to 20 and is capped at 100"`
	MaxOutputBytes int64          `json:"max_output_bytes,omitempty" jsonschema:"Maximum serialized output bytes; defaults to 65536 and is capped by the server response-read limit"`
}

type ExtractQuery struct {
	Name        string `json:"name" jsonschema:"Unique result name, at most 128 UTF-8 bytes"`
	Selector    string `json:"selector" jsonschema:"CSS selector for HTML or RFC 9535 JSONPath expression for JSON"`
	Mode        string `json:"mode,omitempty" jsonschema:"HTML value mode: text, inner_html, outer_html or attribute; defaults to text and is ignored for JSON"`
	Attribute   string `json:"attribute,omitempty" jsonschema:"HTML attribute name when mode is attribute"`
	ResolveURLs bool   `json:"resolve_urls,omitempty" jsonschema:"Resolve relative HTML URL attributes against the stored response final URL"`
}

type ExtractedValue struct {
	Value    string `json:"value"`
	Encoding string `json:"encoding"`
	Path     string `json:"path,omitempty"`
}

type ExtractQueryResult struct {
	Name          string           `json:"name"`
	Selector      string           `json:"selector"`
	Mode          string           `json:"mode"`
	Values        []ExtractedValue `json:"values"`
	MatchedCount  int              `json:"matched_count"`
	ReturnedCount int              `json:"returned_count"`
	Limited       bool             `json:"limited"`
}

type ResponseExtractOutput struct {
	ResponseID      string               `json:"response_id"`
	Format          string               `json:"format"`
	ContentType     string               `json:"content_type,omitempty"`
	FinalURL        string               `json:"final_url,omitempty"`
	SourceBytes     int64                `json:"source_bytes"`
	SourceTruncated bool                 `json:"source_truncated"`
	Results         []ExtractQueryResult `json:"results"`
	ReturnedCount   int                  `json:"returned_count"`
	Limited         bool                 `json:"limited"`
	OutputBytes     int                  `json:"output_bytes"`
}

func (f *Fetcher) ExtractResponse(input ResponseExtractInput) (ResponseExtractOutput, error) {
	record, err := f.response(input.ResponseID)
	if err != nil {
		return ResponseExtractOutput{}, err
	}
	if !utf8.Valid(record.body) {
		return ResponseExtractOutput{}, fmt.Errorf("response_extract requires a UTF-8 HTML or JSON response")
	}
	queries, err := validateExtractQueries(input.Queries)
	if err != nil {
		return ResponseExtractOutput{}, err
	}
	maxResults, err := extractResultLimit(input.MaxResults)
	if err != nil {
		return ResponseExtractOutput{}, err
	}
	maxOutputBytes, err := f.extractOutputLimit(input.MaxOutputBytes)
	if err != nil {
		return ResponseExtractOutput{}, err
	}
	format, err := detectExtractFormat(input.Format, record.contentType, record.body)
	if err != nil {
		return ResponseExtractOutput{}, err
	}

	output := ResponseExtractOutput{
		ResponseID:      input.ResponseID,
		Format:          format,
		ContentType:     record.contentType,
		FinalURL:        record.finalURL,
		SourceBytes:     int64(len(record.body)),
		SourceTruncated: record.truncated,
		Results:         make([]ExtractQueryResult, 0, len(queries)),
	}

	switch format {
	case "html":
		output.Results, err = extractHTML(record, queries, maxResults)
	case "json":
		output.Results, err = extractJSON(record.body, queries, maxResults)
		if err != nil && record.truncated {
			err = fmt.Errorf("%w (the stored JSON response is truncated; fetch it again with a larger max_response_bytes)", err)
		}
	default:
		err = fmt.Errorf("unsupported extraction format %q", format)
	}
	if err != nil {
		return ResponseExtractOutput{}, err
	}
	for _, result := range output.Results {
		output.ReturnedCount += result.ReturnedCount
		output.Limited = output.Limited || result.Limited
	}
	if err := fitExtractionOutput(&output, maxOutputBytes); err != nil {
		return ResponseExtractOutput{}, err
	}
	return output, nil
}

func validateExtractQueries(input []ExtractQuery) ([]ExtractQuery, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("queries must contain at least one extraction query")
	}
	if len(input) > maxExtractQueries {
		return nil, fmt.Errorf("queries must contain at most %d extraction queries", maxExtractQueries)
	}
	queries := make([]ExtractQuery, len(input))
	names := make(map[string]struct{}, len(input))
	for i, query := range input {
		query.Name = strings.TrimSpace(query.Name)
		query.Selector = strings.TrimSpace(query.Selector)
		query.Mode = strings.ToLower(strings.TrimSpace(query.Mode))
		query.Attribute = strings.TrimSpace(query.Attribute)
		if query.Name == "" {
			return nil, fmt.Errorf("queries[%d].name is required", i)
		}
		if !utf8.ValidString(query.Name) || len(query.Name) > maxExtractNameBytes {
			return nil, fmt.Errorf("queries[%d].name must be valid UTF-8 and at most %d bytes", i, maxExtractNameBytes)
		}
		if _, exists := names[query.Name]; exists {
			return nil, fmt.Errorf("queries[%d].name %q is duplicated", i, query.Name)
		}
		names[query.Name] = struct{}{}
		if query.Selector == "" {
			return nil, fmt.Errorf("queries[%d].selector is required", i)
		}
		if !utf8.ValidString(query.Selector) || len(query.Selector) > maxExtractSelectorBytes {
			return nil, fmt.Errorf("queries[%d].selector must be valid UTF-8 and at most %d bytes", i, maxExtractSelectorBytes)
		}
		queries[i] = query
	}
	return queries, nil
}

func extractResultLimit(value int) (int, error) {
	if value == 0 {
		return defaultExtractResults, nil
	}
	if value < 1 || value > maxExtractResults {
		return 0, fmt.Errorf("max_results must be between 1 and %d", maxExtractResults)
	}
	return value, nil
}

func (f *Fetcher) extractOutputLimit(value int64) (int64, error) {
	if value == 0 {
		value = defaultExtractOutputBytes
		if value > f.config.MaxReadBytes {
			value = f.config.MaxReadBytes
		}
	}
	if value < 1 || value > f.config.MaxReadBytes {
		return 0, fmt.Errorf("max_output_bytes must be between 1 and %d", f.config.MaxReadBytes)
	}
	return value, nil
}

func detectExtractFormat(requested, contentType string, body []byte) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "html", "json":
		return requested, nil
	case "", "auto":
	default:
		return "", fmt.Errorf("format must be auto, html or json")
	}

	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		mediaType = strings.ToLower(parsed)
	}
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		return "html", nil
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return "json", nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '{', '[':
			return "json", nil
		case '<':
			return "html", nil
		}
	}
	return "", fmt.Errorf("could not detect HTML or JSON; set format explicitly")
}

func extractHTML(record *storedResponse, queries []ExtractQuery, maxResults int) ([]ExtractQueryResult, error) {
	document, err := html.Parse(bytes.NewReader(record.body))
	if err != nil {
		return nil, fmt.Errorf("parse HTML response: %w", err)
	}
	var baseURL *url.URL
	if record.finalURL != "" {
		baseURL, _ = url.Parse(record.finalURL)
	}
	results := make([]ExtractQueryResult, 0, len(queries))
	for i, query := range queries {
		mode := query.Mode
		if mode == "" {
			mode = "text"
		}
		switch mode {
		case "text", "inner_html", "outer_html":
			if query.Attribute != "" {
				return nil, fmt.Errorf("queries[%d].attribute is only valid when mode is attribute", i)
			}
			if query.ResolveURLs {
				return nil, fmt.Errorf("queries[%d].resolve_urls is only valid when mode is attribute", i)
			}
		case "attribute":
			if query.Attribute == "" {
				return nil, fmt.Errorf("queries[%d].attribute is required when mode is attribute", i)
			}
			if len(query.Attribute) > maxExtractNameBytes {
				return nil, fmt.Errorf("queries[%d].attribute must be at most %d bytes", i, maxExtractNameBytes)
			}
			if query.ResolveURLs {
				if _, supported := htmlURLAttributes[strings.ToLower(query.Attribute)]; !supported {
					return nil, fmt.Errorf("queries[%d].resolve_urls requires a URL attribute such as href, src or action", i)
				}
				if baseURL == nil || !baseURL.IsAbs() {
					return nil, fmt.Errorf("queries[%d].resolve_urls requires an absolute stored final URL", i)
				}
			}
		default:
			return nil, fmt.Errorf("queries[%d].mode must be text, inner_html, outer_html or attribute", i)
		}

		selector, err := cascadia.Compile(query.Selector)
		if err != nil {
			return nil, fmt.Errorf("queries[%d].selector is invalid CSS: %w", i, err)
		}
		nodes := selector.MatchAll(document)
		extractableCount := len(nodes)
		selectedNodes := nodes[:min(len(nodes), maxResults)]
		if mode == "attribute" {
			extractableCount = 0
			selectedNodes = nodes
		}
		values := make([]ExtractedValue, 0, min(len(nodes), maxResults))
		for _, node := range selectedNodes {
			value, exists, err := htmlNodeValue(node, mode, query.Attribute)
			if err != nil {
				return nil, fmt.Errorf("extract queries[%d]: %w", i, err)
			}
			if !exists {
				continue
			}
			if mode == "attribute" {
				extractableCount++
				if len(values) >= maxResults {
					continue
				}
			}
			if query.ResolveURLs {
				reference, err := url.Parse(value)
				if err != nil {
					return nil, fmt.Errorf("extract queries[%d] URL attribute: %w", i, err)
				}
				value = baseURL.ResolveReference(reference).String()
			}
			encoding := "text"
			if mode == "inner_html" || mode == "outer_html" {
				encoding = "html"
			}
			values = append(values, ExtractedValue{Value: value, Encoding: encoding})
		}
		results = append(results, ExtractQueryResult{
			Name:          query.Name,
			Selector:      query.Selector,
			Mode:          mode,
			Values:        values,
			MatchedCount:  len(nodes),
			ReturnedCount: len(values),
			Limited:       extractableCount > maxResults,
		})
	}
	return results, nil
}

func htmlNodeValue(node *html.Node, mode, attribute string) (string, bool, error) {
	switch mode {
	case "text":
		return normalizedNodeText(node), true, nil
	case "inner_html":
		var output strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := html.Render(&output, child); err != nil {
				return "", false, err
			}
		}
		return output.String(), true, nil
	case "outer_html":
		var output strings.Builder
		if err := html.Render(&output, node); err != nil {
			return "", false, err
		}
		return output.String(), true, nil
	case "attribute":
		for _, attr := range node.Attr {
			if strings.EqualFold(attr.Key, attribute) {
				return attr.Val, true, nil
			}
		}
		return "", false, nil
	default:
		return "", false, fmt.Errorf("unsupported HTML extraction mode %q", mode)
	}
}

func normalizedNodeText(node *html.Node) string {
	var parts []string
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			if text := strings.Join(strings.Fields(current.Data), " "); text != "" {
				parts = append(parts, text)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(parts, " ")
}

func extractJSON(body []byte, queries []ExtractQuery, maxResults int) ([]ExtractQueryResult, error) {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse JSON response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}

	results := make([]ExtractQueryResult, 0, len(queries))
	for i, query := range queries {
		if query.Mode != "" || query.Attribute != "" || query.ResolveURLs {
			return nil, fmt.Errorf("queries[%d] HTML mode, attribute and URL options are not valid for JSON extraction", i)
		}
		path, err := jsonpath.Parse(query.Selector)
		if err != nil {
			return nil, fmt.Errorf("queries[%d].selector is invalid RFC 9535 JSONPath: %w", i, err)
		}
		nodes := path.SelectLocated(document)
		limit := min(len(nodes), maxResults)
		values := make([]ExtractedValue, 0, limit)
		for _, node := range nodes[:limit] {
			encoded, err := json.Marshal(node.Node)
			if err != nil {
				return nil, fmt.Errorf("encode queries[%d] result: %w", i, err)
			}
			values = append(values, ExtractedValue{
				Value:    string(encoded),
				Encoding: "json",
				Path:     node.Path.String(),
			})
		}
		results = append(results, ExtractQueryResult{
			Name:          query.Name,
			Selector:      query.Selector,
			Mode:          "json",
			Values:        values,
			MatchedCount:  len(nodes),
			ReturnedCount: len(values),
			Limited:       len(nodes) > maxResults,
		})
	}
	return results, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("parse JSON response: multiple top-level values")
	}
	return fmt.Errorf("parse JSON response: %w", err)
}

func fitExtractionOutput(output *ResponseExtractOutput, maxBytes int64) error {
	for {
		size, err := extractionOutputSize(output)
		if err != nil {
			return err
		}
		if int64(size) <= maxBytes {
			output.OutputBytes = size
			return nil
		}
		removed := false
		for i := len(output.Results) - 1; i >= 0; i-- {
			result := &output.Results[i]
			if len(result.Values) == 0 {
				continue
			}
			result.Values = result.Values[:len(result.Values)-1]
			result.ReturnedCount = len(result.Values)
			result.Limited = true
			output.ReturnedCount--
			output.Limited = true
			removed = true
			break
		}
		if !removed {
			return fmt.Errorf("max_output_bytes=%d is too small for extraction metadata", maxBytes)
		}
	}
}

func extractionOutputSize(output *ResponseExtractOutput) (int, error) {
	previous := -1
	for range 4 {
		encoded, err := json.Marshal(output)
		if err != nil {
			return 0, fmt.Errorf("encode extraction output: %w", err)
		}
		size := len(encoded)
		if size == previous {
			return size, nil
		}
		output.OutputBytes = size
		previous = size
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return 0, fmt.Errorf("encode extraction output: %w", err)
	}
	return len(encoded), nil
}
