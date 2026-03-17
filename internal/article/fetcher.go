// Package article provides utilities for fetching article text from URLs.
package article

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

var (
	archivePHPrefix           = "https://archive.ph/"
	waybackAvailabilityPrefix = "https://archive.org/wayback/available?url="
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
	candidates := []struct {
		source string
		url    func() (string, error)
	}{
		{source: "direct", url: func() (string, error) { return rawURL, nil }},
		{source: "archive.ph", url: func() (string, error) { return archivePHPrefix + rawURL, nil }},
		{source: "internet archive", url: func() (string, error) { return f.internetArchiveSnapshotURL(rawURL) }},
	}

	for _, candidate := range candidates {
		candidateURL, err := candidate.url()
		if err != nil {
			log.Printf("    article fetch skipped (%s): %v", candidate.source, err)
			continue
		}
		log.Printf("    article fetch attempt (%s): %s", candidate.source, candidateURL)
		text, truncated, err := f.fetchURL(candidateURL)
		if err != nil {
			log.Printf("    article fetch failed (%s): %v", candidate.source, err)
			continue
		}
		if len(text) >= 300 {
			if candidate.source != "direct" {
				log.Printf("    article fetch succeeded via fallback: %s", candidate.source)
			}
			return text, truncated, nil
		}
		log.Printf("    article fetch produced insufficient content (%s): %d chars", candidate.source, utf8.RuneCountInString(text))
	}
	return "", false, fmt.Errorf("could not retrieve article content for %s", rawURL)
}

type waybackAvailabilityResponse struct {
	ArchivedSnapshots struct {
		Closest struct {
			Available bool   `json:"available"`
			URL       string `json:"url"`
		} `json:"closest"`
	} `json:"archived_snapshots"`
}

func (f *Fetcher) internetArchiveSnapshotURL(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", waybackAvailabilityPrefix+url.QueryEscape(rawURL), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d for wayback availability lookup", resp.StatusCode)
	}

	var payload waybackAvailabilityResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode wayback availability response: %w", err)
	}

	closest := payload.ArchivedSnapshots.Closest
	if !closest.Available || closest.URL == "" {
		return "", fmt.Errorf("no archived snapshot available")
	}
	return waybackExactReplayURL(closest.URL), nil
}

func waybackExactReplayURL(snapshotURL string) string {
	const marker = "/web/"

	idx := strings.Index(snapshotURL, marker)
	if idx < 0 {
		return snapshotURL
	}

	rest := snapshotURL[idx+len(marker):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return snapshotURL
	}

	timestamp := rest[:slash]
	if timestamp == "" || strings.HasSuffix(timestamp, "id_") {
		return snapshotURL
	}
	for _, r := range timestamp {
		if r < '0' || r > '9' {
			return snapshotURL
		}
	}

	return snapshotURL[:idx+len(marker)] + timestamp + "id_" + snapshotURL[idx+len(marker)+slash:]
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
	// Detect whether the response body was capped at the limit.
	bodyTruncated := int64(len(body)) >= f.maxBodyBytes

	text, textTruncated := extractTextWithLimit(string(body), f.maxTextLen)
	return text, bodyTruncated || textTruncated, nil
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
