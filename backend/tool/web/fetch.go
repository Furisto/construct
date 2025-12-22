package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"

	"github.com/furisto/construct/backend/tool/base"
)

const (
	DefaultTimeout   = 30
	MaxContentSize   = 5 * 1024 * 1024
	DefaultUserAgent = "Mozilla/5.0 (compatible; ConstructBot/1.0)"
)

type FetchInput struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}

type FetchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	ByteSize    int    `json:"byte_size"`
	Truncated   bool   `json:"truncated"`
}

func Fetch(ctx context.Context, client *http.Client, input *FetchInput) (*FetchResult, error) {
	if input.URL == "" {
		return nil, base.NewCustomError("URL is required", []string{
			"Provide a valid HTTP or HTTPS URL",
		})
	}

	parsedURL, err := url.Parse(input.URL)
	if err != nil {
		return nil, base.NewCustomError("Invalid URL", []string{
			"Ensure the URL is properly formatted",
			"Include the protocol (http:// or https://)",
		}, "url", input.URL, "error", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, base.NewCustomError("Invalid URL scheme", []string{
			"Only http:// and https:// URLs are supported",
		}, "scheme", parsedURL.Scheme)
	}

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, base.NewCustomError("Failed to create request", []string{
			"Check that the URL is valid",
		}, "error", err)
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.8,*/*;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	for key, value := range input.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, base.NewCustomError("Request timed out", []string{
				"The server took too long to respond",
				"Try increasing the timeout parameter",
			}, "timeout_seconds", timeout)
		}
		return nil, base.NewCustomError("Failed to fetch URL", []string{
			"Check that the URL is accessible",
			"Verify your network connection",
		}, "url", input.URL, "error", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, base.NewCustomError("HTTP request failed", []string{
			"The server returned an error status",
			"Check that the URL is correct and accessible",
		}, "status_code", resp.StatusCode, "status", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml")
	isJSON := strings.Contains(contentType, "application/json")

	if !isHTML && !isJSON {
		return nil, base.NewCustomError("Unsupported content type", []string{
			"This tool supports HTML web pages and JSON responses",
			"The server returned content type: " + contentType,
		}, "content_type", contentType)
	}

	limitedReader := io.LimitReader(resp.Body, MaxContentSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, base.NewCustomError("Failed to read response body", []string{
			"The server response could not be read",
		}, "error", err)
	}

	truncated := len(body) > MaxContentSize
	if truncated {
		body = body[:MaxContentSize]
	}

	originalSize := len(body)

	var content string
	var title string
	var resultContentType string

	switch {
	case isJSON:
		content = string(body)
		title = parsedURL.Host + parsedURL.Path
		resultContentType = "json"
	case isHTML:
		article, err := readability.FromReader(strings.NewReader(string(body)), parsedURL)
		if err != nil {
			return nil, base.NewCustomError("Failed to extract content from page", []string{
				"The page content could not be parsed",
				"This may happen with non-standard HTML or JavaScript-heavy pages",
			}, "error", err)
		}

		converter := md.NewConverter("", true, nil)
		markdown, err := converter.ConvertString(article.Content)
		if err != nil {
			return nil, base.NewCustomError("Failed to convert content to markdown", []string{
				"The HTML content could not be converted to markdown",
			}, "error", err)
		}

		content = cleanupMarkdown(markdown)
		title = article.Title
		if title == "" {
			title = parsedURL.Host + parsedURL.Path
		}
		resultContentType = "html"
	}

	sizePercent := 0.0
	if originalSize > 0 {
		sizePercent = float64(len(content)) / float64(originalSize) * 100
	}

	slog.DebugContext(ctx, "web fetch completed",
		"url", input.URL,
		"title", title,
		"content_type", resultContentType,
		"original_size", originalSize,
		"compression_ratio", fmt.Sprintf("%.1f%%", sizePercent),
		"truncated", truncated,
	)

	return &FetchResult{
		URL:         input.URL,
		Title:       title,
		Content:     content,
		ContentType: resultContentType,
		ByteSize:    originalSize,
		Truncated:   truncated,
	}, nil
}

func cleanupMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	emptyCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyCount++
			if emptyCount <= 2 {
				result = append(result, "")
			}
		} else {
			emptyCount = 0
			result = append(result, line)
		}
	}

	output := strings.Join(result, "\n")
	output = strings.TrimSpace(output)

	return output
}
