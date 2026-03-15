// Package article provides utilities for fetching article text from URLs.
package article

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	userAgent          = "Mozilla/5.0 (compatible; HNCritique/1.0; +https://github.com/peterwwillis/hn-critique)"
	defaultMaxBodySize = 2 << 20 // 2 MB
	defaultMaxTextLen  = 8000
)

// Limits controls fetcher resource caps.
type Limits struct {
	MaxBodyBytes int64
	MaxTextLen   int
}

// Fetcher retrieves article text from URLs, with paywall bypass fallbacks.
type Fetcher struct {
	http         *http.Client
	maxBodyBytes int64
	maxTextLen   int
}

// NewFetcher returns a new Fetcher.
func NewFetcher() *Fetcher {
	return NewFetcherWithLimits(Limits{})
}

// NewFetcherWithLimits returns a Fetcher configured with custom limits.
func NewFetcherWithLimits(limits Limits) *Fetcher {
	if limits.MaxBodyBytes <= 0 {
		limits.MaxBodyBytes = defaultMaxBodySize
	}
	if limits.MaxTextLen <= 0 {
		limits.MaxTextLen = defaultMaxTextLen
	}
	return &Fetcher{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxBodyBytes: limits.MaxBodyBytes,
		maxTextLen:   limits.MaxTextLen,
	}
}

// Fetch attempts to retrieve readable text from url.
// If the direct fetch appears paywalled or fails, it tries archive.ph and the
// Wayback Machine before giving up.
func (f *Fetcher) Fetch(rawURL string) (string, error) {
	candidates := []string{
		rawURL,
		"https://archive.ph/" + rawURL,
		"https://web.archive.org/web/newest/" + rawURL,
	}

	for _, u := range candidates {
		text, err := f.fetchURL(u)
		if err == nil && len(text) >= 300 {
			return text, nil
		}
	}
	return "", fmt.Errorf("could not retrieve article content for %s", rawURL)
}

func (f *Fetcher) fetchURL(u string) (string, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodyBytes))
	if err != nil {
		return "", err
	}

	text := extractTextWithLimit(string(body), f.maxTextLen)
	return text, nil
}

// ExtractText parses HTML and returns the visible text content, up to the default limit.
func ExtractText(htmlContent string) string {
	return extractTextWithLimit(htmlContent, defaultMaxTextLen)
}

// extractTextWithLimit is the internal implementation with an explicit limit.
func extractTextWithLimit(htmlContent string, maxTextLen int) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		// Treat as plain text
		t := strings.TrimSpace(htmlContent)
		if len(t) > maxTextLen {
			t = t[:maxTextLen] + "…"
		}
		return t
	}

	// Prefer <article> or <main> content nodes for cleaner text.
	if node := findContentNode(doc); node != nil {
		text := nodeText(node)
		if len(text) >= 300 {
			if len(text) > maxTextLen {
				text = text[:maxTextLen] + "…"
			}
			return text
		}
	}

	text := nodeText(doc)
	if len(text) > maxTextLen {
		text = text[:maxTextLen] + "…"
	}
	return text
}

// findContentNode locates the best semantic content node in the document.
func findContentNode(n *html.Node) *html.Node {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "article", "main":
			return n
		}
		for _, a := range n.Attr {
			if a.Key == "role" && a.Val == "main" {
				return n
			}
			if a.Key == "class" {
				for _, cls := range strings.Fields(a.Val) {
					switch cls {
					case "article-body", "post-content", "entry-content",
						"content-body", "article__body", "story-body":
						return n
					}
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findContentNode(c); found != nil {
			return found
		}
	}
	return nil
}

// nodeText returns the concatenated visible text of a node, skipping non-content elements.
func nodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "nav", "footer",
				"header", "aside", "form", "iframe", "button":
				return
			}
		}
		if node.Type == html.TextNode {
			t := strings.TrimSpace(node.Data)
			if t != "" {
				sb.WriteString(t)
				sb.WriteByte(' ')
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr":
				sb.WriteByte('\n')
			}
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}
