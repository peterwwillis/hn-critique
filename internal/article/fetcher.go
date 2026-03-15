// Package article provides utilities for fetching article text from URLs.
package article

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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
// This method preserves the original API and returns only the extracted text
// and an error. To also know whether the text was truncated at the configured
// character limit, use FetchWithTruncation instead.
func (f *Fetcher) Fetch(rawURL string) (string, error) {
	text, _, err := f.FetchWithTruncation(rawURL)
	return text, err
}

// FetchWithTruncation attempts to retrieve readable text from url.
// If the direct fetch appears paywalled or fails, it tries archive.ph and the
// Wayback Machine before giving up.
// The second return value is true when the extracted text was truncated at the
// configured character limit, meaning the critique may be incomplete.
func (f *Fetcher) FetchWithTruncation(rawURL string) (string, bool, error) {
	candidates := []string{
		rawURL,
		"https://archive.ph/" + rawURL,
		"https://web.archive.org/web/newest/" + rawURL,
	}

	for _, u := range candidates {
		text, truncated, err := f.fetchURL(u)
		if err == nil && len(text) >= 300 {
			return text, truncated, nil
		}
	}
	return "", false, fmt.Errorf("could not retrieve article content for %s", rawURL)
}

func (f *Fetcher) fetchURL(u string) (string, bool, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := f.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodyBytes))
	if err != nil {
		return "", false, err
	}

	text, truncated := extractTextWithLimit(string(body), f.maxTextLen)
	return text, truncated, nil
}

// ExtractText parses HTML and returns the visible text content, up to the default limit.
func ExtractText(htmlContent string) string {
	text, _ := extractTextWithLimit(htmlContent, defaultMaxTextLen)
	return text
}

// truncateWithEllipsisUTF8 truncates s so that the result is valid UTF-8 and
// its byte length does not exceed max. When truncation occurs and there is
// sufficient space, it appends an ellipsis ("…").
func truncateWithEllipsisUTF8(s string, max int) (string, bool) {
	if max <= 0 {
		return "", len(s) > 0
	}
	if len(s) <= max {
		return s, false
	}

	const ellipsis = "…"
	ellipsisLen := len(ellipsis)

	// If the ellipsis itself does not fit alongside any content, just return
	// the largest valid UTF-8 prefix that fits within max bytes.
	if ellipsisLen >= max {
		byteCount := 0
		lastGood := 0
		for _, r := range s {
			rLen := utf8.RuneLen(r)
			if rLen < 0 {
				// Should not happen for valid UTF-8, but be defensive.
				rLen = 1
			}
			if byteCount+rLen > max {
				break
			}
			byteCount += rLen
			lastGood = byteCount
		}
		return s[:lastGood], true
	}

	limit := max - ellipsisLen
	byteCount := 0
	lastGood := 0
	for _, r := range s {
		rLen := utf8.RuneLen(r)
		if rLen < 0 {
			rLen = 1
		}
		if byteCount+rLen > limit {
			break
		}
		byteCount += rLen
		lastGood = byteCount
	}

	return s[:lastGood] + ellipsis, true
}

// extractTextWithLimit is the internal implementation with an explicit limit.
// It returns the extracted text and a boolean that is true when the text was
// truncated to fit within maxTextLen.
func extractTextWithLimit(htmlContent string, maxTextLen int) (string, bool) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		// Treat as plain text
		t := strings.TrimSpace(htmlContent)
		return truncateWithEllipsisUTF8(t, maxTextLen)
	}

	// Prefer <article> or <main> content nodes for cleaner text.
	if node := findContentNode(doc); node != nil {
		text := nodeText(node)
		if len(text) >= 300 {
			return truncateWithEllipsisUTF8(text, maxTextLen)
		}
	}

	text := nodeText(doc)
	return truncateWithEllipsisUTF8(text, maxTextLen)
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
