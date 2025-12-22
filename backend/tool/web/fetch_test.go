package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestFetch(t *testing.T) {
	setup := &base.ToolTestSetup[*FetchInput, *FetchResult]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *FetchInput) (*FetchResult, error) {
			return Fetch(ctx, http.DefaultClient, input)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(FetchResult{}, "URL", "Content", "ByteSize"),
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions", "Details"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*FetchInput, *FetchResult]{
		{
			Name: "successful fetch of HTML page",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/html")
					w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<article>
<h1>Hello World</h1>
<p>This is a test paragraph.</p>
</article>
</body>
</html>`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Result: &FetchResult{
					Title:     "Test Page",
					Truncated: false,
				},
			},
		},
		{
			Name:      "empty URL error",
			TestInput: &FetchInput{URL: ""},
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("URL is required", nil),
			},
		},
		{
			Name:      "invalid URL scheme",
			TestInput: &FetchInput{URL: "ftp://example.com"},
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("Invalid URL scheme", nil),
			},
		},
		{
			Name:      "missing URL scheme",
			TestInput: &FetchInput{URL: "example.com/page"},
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("Invalid URL scheme", nil),
			},
		},
		{
			Name: "HTTP 404 error",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("HTTP request failed", nil),
			},
		},
		{
			Name: "HTTP 500 error",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("HTTP request failed", nil),
			},
		},
		{
			Name: "non-HTML content type",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"key": "value"}`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Error: base.NewCustomError("URL does not return HTML content", nil),
			},
		},
		{
			Name: "custom headers are sent",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer token123" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					if r.Header.Get("X-Custom") != "value" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "text/html")
					w.Write([]byte(`<!DOCTYPE html><html><head><title>Auth Page</title></head><body><p>Authenticated</p></body></html>`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{
					URL: server.URL,
					Headers: map[string]string{
						"Authorization": "Bearer token123",
						"X-Custom":      "value",
					},
				}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Result: &FetchResult{
					Title:     "Auth Page",
					Truncated: false,
				},
			},
		},
		{
			Name: "default user agent is set",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("User-Agent") != DefaultUserAgent {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "text/html")
					w.Write([]byte(`<!DOCTYPE html><html><head><title>UA Test</title></head><body><p>OK</p></body></html>`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Result: &FetchResult{
					Title:     "UA Test",
					Truncated: false,
				},
			},
		},
		{
			Name: "large content is truncated",
			TestInput: func() *FetchInput {
				largeBody := strings.Repeat("<p>content</p>", MaxContentSize/10)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/html")
					w.Write([]byte(`<!DOCTYPE html><html><head><title>Large Page</title></head><body>` + largeBody + `</body></html>`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Result: &FetchResult{
					Title:     "Large Page",
					Truncated: true,
				},
			},
		},
		{
			Name: "xhtml content type accepted",
			TestInput: func() *FetchInput {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/xhtml+xml")
					w.Write([]byte(`<!DOCTYPE html><html><head><title>XHTML Page</title></head><body><p>XHTML content</p></body></html>`))
				}))
				t.Cleanup(server.Close)
				return &FetchInput{URL: server.URL}
			}(),
			Expected: base.ToolTestExpectation[*FetchResult]{
				Result: &FetchResult{
					Title:     "XHTML Page",
					Truncated: false,
				},
			},
		},
	})
}
