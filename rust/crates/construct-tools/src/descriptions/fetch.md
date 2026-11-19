
## Description
Fetches a web page or API endpoint. Returns Markdown for HTML pages and formatted JSON for API responses.

## Parameters
- **url** (*string*, required): The URL to fetch. Must be an HTTP or HTTPS URL.
- **headers** (*object*, optional): Custom HTTP headers to include in the request. Useful for authentication or setting custom User-Agent.
- **timeout** (*number*, optional): Request timeout in seconds. Defaults to 30 seconds.

## Expected Output
Returns an object containing the fetched content:
```
{
  "url": "https://example.com/article",
  "title": "Article Title",
  "content": "# Article Title

The main content in Markdown format...",
  "byte_size": 15234,
  "truncated": false
}
```

**Details:**
- **url**: The URL that was fetched
- **title**: The page title extracted from HTML (empty for JSON responses)
- **content**: The main content - Markdown for HTML pages, formatted JSON for API responses
- **byte_size**: Size of the original content in bytes
- **truncated**: Whether the content was truncated due to size limits (max 5MB)

## CRITICAL REQUIREMENTS
- **URL Validation**: The URL must include a valid protocol (http:// or https://).
  ```
  // Correct
  fetch({ url: "https://docs.example.com/guide" })
  
  // Incorrect - missing protocol
  fetch({ url: "docs.example.com/guide" })
  ```
- **Supported Content Types**: This tool works with HTML web pages and JSON APIs. It will return an error for PDFs, images, or other binary content.
- **No JavaScript Rendering**: Pages that require JavaScript to render content (SPAs, React apps) may return incomplete or empty content.
- **Size Limits**: Content larger than 5MB will be truncated. Check the ```truncated``` field in the response.

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
```
const result = fetch({ url: "https://go.dev/doc/tutorial/getting-started" });
print("Title:", result.title);
print("Content:", result.content);
```

### With custom headers
```
const result = fetch({
  url: "https://api.example.com/docs",
  headers: {
    "Authorization": "Bearer token123",
    "Accept-Language": "en-US"
  }
});
```

### With timeout
```
const result = fetch({
  url: "https://slow-server.example.com/page",
  timeout: 60
});
```

### Fetching JSON API
```
const result = fetch({ url: "https://api.example.com/users/123" });
print("Response:", result.content);
```
