// Package article provides utilities for fetching article text from URLs.
package article

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
	archivePHBaseURL           = "https://archive.ph/"
	waybackAvailabilityAPIURL  = "https://archive.org/wayback/available"
	waybackCDXAPIURL           = "https://web.archive.org/cdx/search/cdx"
	waybackReplayBaseURL       = "https://web.archive.org/"
	errPlaywrightNotConfigured = errors.New("playwright fetch service not configured")
	articleLogger              = log.Default()
)

// Limits controls fetcher resource caps.
type Limits struct {
	MaxBodyBytes int64
	MaxTextLen   int
}

// Fetcher retrieves article text from URLs, with paywall bypass fallbacks.
type Fetcher struct {
	http                 *http.Client
	maxBodyBytes         int64
	maxTextLen           int
	playwrightServiceURL string
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
		maxBodyBytes:         limits.MaxBodyBytes,
		maxTextLen:           limits.MaxTextLen,
		playwrightServiceURL: strings.TrimSpace(os.Getenv("PLAYWRIGHT_FETCH_URL")),
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
		fetch  func() (string, string, bool, error)
	}{
		{source: "direct", fetch: func() (string, string, bool, error) {
			text, truncated, err := f.fetchURL(rawURL)
			return rawURL, text, truncated, err
		}},
		{source: "playwright", fetch: func() (string, string, bool, error) {
			text, truncated, err := f.fetchViaPlaywright(rawURL)
			return f.playwrightServiceURL, text, truncated, err
		}},
		{source: "archive.ph", fetch: func() (string, string, bool, error) {
			snapshotURL, err := f.archivePHSnapshotURL(rawURL)
			if err != nil {
				return "", "", false, err
			}
			text, truncated, err := f.fetchURL(snapshotURL)
			return snapshotURL, text, truncated, err
		}},
		{source: "internet archive", fetch: func() (string, string, bool, error) {
			snapshotURL, err := f.internetArchiveSnapshotURL(rawURL)
			if err != nil {
				return "", "", false, err
			}
			text, truncated, err := f.fetchURL(snapshotURL)
			return snapshotURL, text, truncated, err
		}},
	}

	for _, candidate := range candidates {
		targetURL, text, truncated, err := candidate.fetch()
		if targetURL == "" {
			targetURL = rawURL
		}
		articleLogger.Printf("    article fetch attempt (%s): %s", candidate.source, targetURL)
		if err != nil {
			if errors.Is(err, errPlaywrightNotConfigured) {
				articleLogger.Printf("    article fetch skipped (%s): %v", candidate.source, err)
			} else {
				articleLogger.Printf("    article fetch failed (%s): %v", candidate.source, err)
			}
			continue
		}
		if len(text) >= 300 {
			if candidate.source != "direct" {
				articleLogger.Printf("    article fetch succeeded via fallback: %s", candidate.source)
			}
			return text, truncated, nil
		}
		articleLogger.Printf("    article fetch produced insufficient content (%s): %d chars", candidate.source, utf8.RuneCountInString(text))
	}
	return "", false, fmt.Errorf("could not retrieve article content for %s", rawURL)
}

func (f *Fetcher) fetchViaPlaywright(rawURL string) (string, bool, error) {
	if f.playwrightServiceURL == "" {
		return "", false, errPlaywrightNotConfigured
	}

	body, err := json.Marshal(map[string]string{"url": rawURL})
	if err != nil {
		return "", false, err
	}

	req, err := http.NewRequest("POST", f.playwrightServiceURL, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")

	resp, err := f.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", false, fmt.Errorf("HTTP %d from playwright fetch service: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	htmlBody, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodyBytes))
	if err != nil {
		return "", false, err
	}
	bodyTruncated := int64(len(htmlBody)) >= f.maxBodyBytes
	text, textTruncated := extractTextWithLimit(string(htmlBody), f.maxTextLen)
	return text, bodyTruncated || textTruncated, nil
}

type waybackAvailabilityResponse struct {
	ArchivedSnapshots struct {
		Closest struct {
			Available bool   `json:"available"`
			Status    string `json:"status"`
			URL       string `json:"url"`
		} `json:"closest"`
	} `json:"archived_snapshots"`
}

func (f *Fetcher) archivePHSnapshotURL(rawURL string) (string, error) {
	submitURL, err := url.Parse(archivePHBaseURL)
	if err != nil {
		return "", err
	}
	submitURL = submitURL.ResolveReference(&url.URL{Path: "submit/"})

	form := url.Values{
		"url":    {rawURL},
		"anyway": {"1"},
	}

	req, err := http.NewRequest("POST", submitURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{
		Timeout: f.http.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if transport, ok := f.http.Transport.(*http.Transport); ok && transport != nil {
		client.Transport = transport.Clone()
	} else if f.http.Transport != nil {
		client.Transport = f.http.Transport
	}
	if f.http.Jar != nil {
		client.Jar = f.http.Jar
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d for archive.ph submit", resp.StatusCode)
	}

	snapshotURL, ok := archivePHResponseURL(submitURL, resp.Header)
	if !ok {
		return "", fmt.Errorf("archive.ph submit response did not provide snapshot URL")
	}
	return archivePHNormalizeSnapshotURL(snapshotURL), nil
}

func (f *Fetcher) internetArchiveSnapshotURL(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", waybackAvailabilityAPIURL, nil)
	if err != nil {
		return "", err
	}
	query := req.URL.Query()
	query.Set("url", rawURL)
	query.Set("timestamp", time.Now().UTC().Format("20060102150405"))
	req.URL.RawQuery = query.Encode()
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
	if closest.Available && closest.URL != "" && (closest.Status == "" || closest.Status == "200") {
		return normalizeInternetArchiveSnapshotURL(closest.URL), nil
	}
	return f.internetArchiveCDXSnapshotURL(rawURL)
}

func (f *Fetcher) internetArchiveCDXSnapshotURL(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", waybackCDXAPIURL, nil)
	if err != nil {
		return "", err
	}
	query := req.URL.Query()
	query.Set("url", rawURL)
	query.Set("output", "json")
	query.Set("fl", "timestamp,original")
	query.Add("filter", "statuscode:200")
	query.Add("filter", "mimetype:text/html")
	query.Set("limit", "1")
	query.Set("sort", "reverse")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d for wayback CDX lookup", resp.StatusCode)
	}

	var rows [][]string
	// CDX responses are small metadata payloads; 1 MiB is ample for a single URL lookup
	// while still avoiding unbounded reads on an external service response.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rows); err != nil {
		return "", fmt.Errorf("decode wayback CDX response: %w", err)
	}
	if len(rows) < 2 || len(rows[1]) < 2 {
		return "", fmt.Errorf("no archived snapshot available")
	}

	timestamp := strings.TrimSpace(rows[1][0])
	original := strings.TrimSpace(rows[1][1])
	if timestamp == "" || original == "" {
		return "", fmt.Errorf("no archived snapshot available")
	}

	return normalizeInternetArchiveSnapshotURL(internetArchiveReplayURL(timestamp, original)), nil
}

func internetArchiveReplayURL(timestamp, original string) string {
	return strings.TrimRight(waybackReplayBaseURL, "/") + "/web/" + timestamp + "/" + original
}

func normalizeInternetArchiveSnapshotURL(snapshotURL string) string {
	parsed, err := url.Parse(snapshotURL)
	if err != nil {
		return snapshotURL
	}
	if parsed.Host == "web.archive.org" && parsed.Scheme == "http" {
		parsed.Scheme = "https"
	}
	return parsed.String()
}

func archivePHResponseURL(base *url.URL, headers http.Header) (string, bool) {
	for _, key := range []string{"Refresh", "Location"} {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			continue
		}
		if key == "Refresh" {
			lowerValue := strings.ToLower(value)
			idx := strings.Index(lowerValue, ";url=")
			if idx < 0 {
				continue
			}
			value = strings.TrimSpace(value[idx+len(";url="):])
		}
		parsed, err := url.Parse(value)
		if err != nil {
			continue
		}
		return base.ResolveReference(parsed).String(), true
	}
	return "", false
}

func archivePHNormalizeSnapshotURL(snapshotURL string) string {
	parsed, err := url.Parse(snapshotURL)
	if err != nil {
		return snapshotURL
	}
	parsed.Path = strings.Replace(parsed.Path, "/wip/", "/", 1)
	return parsed.String()
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
