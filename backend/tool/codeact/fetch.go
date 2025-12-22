package codeact

import (
	"fmt"
	"net/http"

	"github.com/furisto/construct/backend/tool/web"
	"github.com/grafana/sobek"
)

var fetchDescription = `
## Description
Fetches a web page or API endpoint. Returns Markdown for HTML pages and formatted JSON for API responses.

## Parameters
- **url** (*string*, required): The URL to fetch. Must be an HTTP or HTTPS URL.
- **headers** (*object*, optional): Custom HTTP headers to include in the request. Useful for authentication or setting custom User-Agent.
- **timeout** (*number*, optional): Request timeout in seconds. Defaults to 30 seconds.

## Expected Output
Returns an object containing the fetched content:
%[1]s
{
  "url": "https://example.com/article",
  "title": "Article Title",
  "content": "# Article Title

The main content in Markdown format...",
  "byte_size": 15234,
  "truncated": false
}
%[1]s

**Details:**
- **url**: The URL that was fetched
- **title**: The page title extracted from HTML (empty for JSON responses)
- **content**: The main content - Markdown for HTML pages, formatted JSON for API responses
- **byte_size**: Size of the original content in bytes
- **truncated**: Whether the content was truncated due to size limits (max 5MB)

## CRITICAL REQUIREMENTS
- **URL Validation**: The URL must include a valid protocol (http:// or https://).
  %[1]s
  // Correct
  fetch({ url: "https://docs.example.com/guide" })
  
  // Incorrect - missing protocol
  fetch({ url: "docs.example.com/guide" })
  %[1]s
- **Supported Content Types**: This tool works with HTML web pages and JSON APIs. It will return an error for PDFs, images, or other binary content.
- **No JavaScript Rendering**: Pages that require JavaScript to render content (SPAs, React apps) may return incomplete or empty content.
- **Size Limits**: Content larger than 5MB will be truncated. Check the %[1]struncated%[1]s field in the response.

## When to use
- **Documentation Lookup**: Fetching API documentation, library docs, or technical references.
- **Article Reading**: Retrieving blog posts, news articles, or wiki pages.
- **Research**: Gathering information from web pages for analysis.
- **Content Extraction**: Getting clean text from web pages without HTML markup.
- **API Requests**: Fetching data from JSON APIs and REST endpoints.

## Limitations
- Does not execute JavaScript - content rendered client-side may be missing
- May not work with pages that require authentication (unless headers are provided)
- Some sites may block automated requests
- Does not follow redirects beyond standard HTTP redirects

## Usage Examples

### Basic fetch
%[1]s
const result = fetch({ url: "https://go.dev/doc/tutorial/getting-started" });
print("Title:", result.title);
print("Content:", result.content);
%[1]s

### With custom headers
%[1]s
const result = fetch({
  url: "https://api.example.com/docs",
  headers: {
    "Authorization": "Bearer token123",
    "Accept-Language": "en-US"
  }
});
%[1]s

### With timeout
%[1]s
const result = fetch({
  url: "https://slow-server.example.com/page",
  timeout: 60
});
%[1]s

### Fetching JSON API
%[1]s
const result = fetch({ url: "https://api.example.com/users/123" });
print("Response:", result.content);
%[1]s
`

func NewFetchTool() Tool {
	return NewOnDemandTool(
		"fetch",
		fmt.Sprintf(fetchDescription, "```"),
		fetchInput,
		fetchHandler,
	)
}

func fetchInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, nil
	}

	inputObj := args[0].ToObject(session.VM)
	if inputObj == nil {
		return nil, nil
	}

	input := &web.FetchInput{}

	if url := inputObj.Get("url"); url != nil && !sobek.IsUndefined(url) {
		input.URL = url.String()
	}

	if timeout := inputObj.Get("timeout"); timeout != nil && !sobek.IsUndefined(timeout) {
		input.Timeout = int(timeout.ToInteger())
	}

	if headers := inputObj.Get("headers"); headers != nil && !sobek.IsUndefined(headers) {
		headersObj := headers.ToObject(session.VM)
		if headersObj != nil {
			input.Headers = make(map[string]string)
			for _, key := range headersObj.Keys() {
				val := headersObj.Get(key)
				if val != nil && !sobek.IsUndefined(val) {
					input.Headers[key] = val.String()
				}
			}
		}
	}

	return input, nil
}

func fetchHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		rawInput, err := fetchInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}

		input := rawInput.(*web.FetchInput)

		client := &http.Client{}

		result, err := web.Fetch(session.Context, client, input)
		if err != nil {
			session.Throw(err)
		}

		SetValue(session, "result", result)
		return session.VM.ToValue(result)
	}
}
