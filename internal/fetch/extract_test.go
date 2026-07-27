package fetch

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractResponseHTML(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`<!doctype html>
<html>
<head><title>Example Catalog</title></head>
<body>
  <main>
    <article class="item"><a href="/items/1"><span>First Item</span></a><strong>€10</strong></article>
    <article class="item"><a href="/items/2">Second Item</a><strong>€20</strong></article>
    <article class="item"><a>Missing URL</a><strong>€30</strong></article>
    <nav><a>Missing URL</a><a href="/next">Next</a><a href="/last">Last</a></nav>
  </main>
</body>
</html>`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "text/html; charset=utf-8",
		FinalURL:     "https://example.com/catalog/page",
	})

	output, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID: responseID,
		Queries: []ExtractQuery{
			{Name: "title", Selector: "title"},
			{Name: "prices", Selector: ".item strong"},
			{Name: "links", Selector: ".item a", Mode: "attribute", Attribute: "href", ResolveURLs: true},
			{Name: "nav_links", Selector: "nav a", Mode: "attribute", Attribute: "href", ResolveURLs: true},
			{Name: "cards", Selector: ".item", Mode: "inner_html"},
		},
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("ExtractResponse() error = %v", err)
	}
	if output.Format != "html" || output.SourceBytes == 0 || output.OutputBytes == 0 {
		t.Fatalf("output metadata = %+v", output)
	}
	if got := output.Results[0].Values[0].Value; got != "Example Catalog" {
		t.Fatalf("title = %q", got)
	}
	if got := valuesOf(output.Results[1]); len(got) != 2 || got[0] != "€10" || got[1] != "€20" {
		t.Fatalf("prices = %v", got)
	}
	links := output.Results[2]
	if links.MatchedCount != 3 || links.ReturnedCount != 2 || links.Limited {
		t.Fatalf("links = %+v", links)
	}
	if got := links.Values[0].Value; got != "https://example.com/items/1" {
		t.Fatalf("first resolved link = %q", got)
	}
	navLinks := output.Results[3]
	if navLinks.MatchedCount != 3 || navLinks.ReturnedCount != 2 || navLinks.Limited {
		t.Fatalf("nav links = %+v", navLinks)
	}
	if got := navLinks.Values[0].Value; got != "https://example.com/next" {
		t.Fatalf("first resolved nav link = %q", got)
	}
	if !strings.Contains(output.Results[4].Values[0].Value, "<a href=\"/items/1\">") {
		t.Fatalf("inner HTML = %q", output.Results[4].Values[0].Value)
	}
	if !output.Limited {
		t.Fatal("output.Limited = false, want true")
	}
}

func TestExtractResponseJSONPath(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`{
  "items": [
    {"id": 101, "title": "First", "price": {"amount": 10}},
    {"id": 102, "title": "Second", "price": {"amount": 20}}
  ],
  "pagination": {"current_page": 1}
}`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "application/json",
		FinalURL:     "https://example.com/api/catalog",
	})

	output, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID: responseID,
		Format:     "auto",
		Queries: []ExtractQuery{
			{Name: "titles", Selector: "$.items[*].title"},
			{Name: "cheap_items", Selector: "$.items[?@.price.amount < 20]"},
			{Name: "page", Selector: "$.pagination.current_page"},
		},
	})
	if err != nil {
		t.Fatalf("ExtractResponse() error = %v", err)
	}
	if output.Format != "json" || output.ReturnedCount != 4 {
		t.Fatalf("output = %+v", output)
	}
	titles := output.Results[0]
	if titles.MatchedCount != 2 ||
		titles.Values[0].Value != `"First"` ||
		titles.Values[1].Value != `"Second"` {
		t.Fatalf("titles = %+v", titles)
	}
	if titles.Values[0].Path != "$['items'][0]['title']" {
		t.Fatalf("first normalized path = %q", titles.Values[0].Path)
	}
	if got := output.Results[1].Values[0].Value; !strings.Contains(got, `"id":101`) {
		t.Fatalf("filtered item = %q", got)
	}
	if got := output.Results[2].Values[0].Value; got != "1" {
		t.Fatalf("page = %q", got)
	}
}

func TestExtractResponseAutoDetectsBody(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`{"ok":true}`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "text/plain",
	})
	output, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID: responseID,
		Queries:    []ExtractQuery{{Name: "ok", Selector: "$.ok"}},
	})
	if err != nil {
		t.Fatalf("ExtractResponse() error = %v", err)
	}
	if output.Format != "json" || output.Results[0].Values[0].Value != "true" {
		t.Fatalf("output = %+v", output)
	}
}

func TestExtractResponsePreservesLargeJSONNumbers(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`{"id":9007199254740993}`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "application/json",
	})

	output, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID: responseID,
		Queries:    []ExtractQuery{{Name: "id", Selector: "$.id"}},
	})
	if err != nil {
		t.Fatalf("ExtractResponse() error = %v", err)
	}
	if got := output.Results[0].Values[0].Value; got != "9007199254740993" {
		t.Fatalf("large integer = %q", got)
	}
}

func TestExtractResponseExplainsTruncatedJSON(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`{"items":[`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "application/json",
		Truncated:    true,
	})

	_, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID: responseID,
		Queries:    []ExtractQuery{{Name: "items", Selector: "$.items[*]"}},
	})
	if err == nil || !strings.Contains(err.Error(), "stored JSON response is truncated") {
		t.Fatalf("ExtractResponse() error = %v, want truncated JSON guidance", err)
	}
}

func TestExtractResponseFitsOutputBudget(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	responseID := storeTestResponse(t, fetcher, []byte(`{
  "items": [
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
  ]
}`), storedResponseMetadata{BodyEncoding: "utf-8", ContentType: "application/json"})

	output, err := fetcher.ExtractResponse(ResponseExtractInput{
		ResponseID:     responseID,
		Queries:        []ExtractQuery{{Name: "items", Selector: "$.items[*]"}},
		MaxOutputBytes: 500,
	})
	if err != nil {
		t.Fatalf("ExtractResponse() error = %v", err)
	}
	if output.OutputBytes > 500 || !output.Limited || output.ReturnedCount >= 3 {
		t.Fatalf("output = %+v", output)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal(output) error = %v", err)
	}
	if output.OutputBytes != len(encoded) {
		t.Fatalf("output_bytes = %d, serialized size = %d", output.OutputBytes, len(encoded))
	}
}

func TestExtractResponseValidation(t *testing.T) {
	fetcher := New(Config{MaxReadBytes: 64 * 1024})
	htmlID := storeTestResponse(t, fetcher, []byte(`<a href="/item">Item</a>`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "text/html",
		FinalURL:     "https://example.com/",
	})
	jsonID := storeTestResponse(t, fetcher, []byte(`{"items":[]}`), storedResponseMetadata{
		BodyEncoding: "utf-8",
		ContentType:  "application/json",
	})
	binaryID := storeTestResponse(t, fetcher, []byte{0xff, 0xfe}, storedResponseMetadata{
		BodyEncoding: "base64",
		ContentType:  "application/octet-stream",
	})

	tests := []struct {
		name  string
		input ResponseExtractInput
		want  string
	}{
		{
			name:  "queries required",
			input: ResponseExtractInput{ResponseID: htmlID},
			want:  "at least one",
		},
		{
			name: "duplicate names",
			input: ResponseExtractInput{
				ResponseID: htmlID,
				Queries: []ExtractQuery{
					{Name: "same", Selector: "a"},
					{Name: "same", Selector: "body"},
				},
			},
			want: "duplicated",
		},
		{
			name: "invalid CSS",
			input: ResponseExtractInput{
				ResponseID: htmlID,
				Queries:    []ExtractQuery{{Name: "bad", Selector: "["}},
			},
			want: "invalid CSS",
		},
		{
			name: "missing attribute",
			input: ResponseExtractInput{
				ResponseID: htmlID,
				Queries:    []ExtractQuery{{Name: "bad", Selector: "a", Mode: "attribute"}},
			},
			want: "attribute is required",
		},
		{
			name: "invalid JSONPath",
			input: ResponseExtractInput{
				ResponseID: jsonID,
				Queries:    []ExtractQuery{{Name: "bad", Selector: "items[*]"}},
			},
			want: "invalid RFC 9535 JSONPath",
		},
		{
			name: "HTML options on JSON",
			input: ResponseExtractInput{
				ResponseID: jsonID,
				Queries:    []ExtractQuery{{Name: "bad", Selector: "$.items", Mode: "text"}},
			},
			want: "not valid for JSON",
		},
		{
			name: "binary response",
			input: ResponseExtractInput{
				ResponseID: binaryID,
				Format:     "json",
				Queries:    []ExtractQuery{{Name: "bad", Selector: "$"}},
			},
			want: "requires a UTF-8",
		},
		{
			name: "output over server cap",
			input: ResponseExtractInput{
				ResponseID:     jsonID,
				Queries:        []ExtractQuery{{Name: "items", Selector: "$.items"}},
				MaxOutputBytes: 64*1024 + 1,
			},
			want: "max_output_bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := fetcher.ExtractResponse(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ExtractResponse() error = %v, want %q", err, test.want)
			}
		})
	}
}

func storeTestResponse(t *testing.T, fetcher *Fetcher, body []byte, metadata storedResponseMetadata) string {
	t.Helper()
	responseID, err := fetcher.storeResponse(body, metadata)
	if err != nil {
		t.Fatalf("storeResponse() error = %v", err)
	}
	return responseID
}

func valuesOf(result ExtractQueryResult) []string {
	values := make([]string, 0, len(result.Values))
	for _, value := range result.Values {
		values = append(values, value.Value)
	}
	return values
}
