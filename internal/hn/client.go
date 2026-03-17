// Package hn provides a client for the Hacker News Firebase API.
package hn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	topStoriesPageURLDefault   = "https://news.ycombinator.com/"
	topStoriesFetchAttempts    = 3
	maxPlaywrightResponseBytes = 2 << 20
)

var (
	baseURL                    = "https://hacker-news.firebaseio.com/v0"
	topStoriesPageURL          = topStoriesPageURLDefault
	errPlaywrightNotConfigured = errors.New("playwright fetch service not configured")
)

// Client is a Hacker News API client.
type Client struct {
	http                 *http.Client
	playwrightServiceURL string
}

// NewClient returns a new HN API client.
func NewClient() *Client {
	return &Client{
		http:                 &http.Client{Timeout: 30 * time.Second},
		playwrightServiceURL: strings.TrimSpace(os.Getenv("PLAYWRIGHT_FETCH_URL")),
	}
}

// Item represents a Hacker News item (story, comment, etc.).
type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Title       string `json:"title"`
	Kids        []int  `json:"kids"`
	Descendants int    `json:"descendants"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
	Parent      int    `json:"parent"`
}

// GetTopStories returns the IDs of the top stories, up to limit.
func (c *Client) GetTopStories(limit int) ([]int, error) {
	var lastErr error
	for attempt := 0; attempt < topStoriesFetchAttempts; attempt++ {
		ids, err := c.getTopStoriesDirect(limit)
		if err == nil {
			return ids, nil
		}
		lastErr = err
	}

	ids, err := c.getTopStoriesViaPlaywright(limit)
	if err == nil {
		return ids, nil
	}
	if errors.Is(err, errPlaywrightNotConfigured) {
		return nil, fmt.Errorf("fetching top stories: %w", lastErr)
	}
	joinedErr := errors.Join(lastErr, err)
	return nil, fmt.Errorf("fetching top stories: direct API failed after %d attempts; playwright fallback failed: %w", topStoriesFetchAttempts, joinedErr)
}

func (c *Client) getTopStoriesDirect(limit int) ([]int, error) {
	resp, err := c.http.Get(baseURL + "/topstories.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from top stories API", resp.StatusCode)
	}

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decoding top stories: %w", err)
	}
	return limitTopStoryIDs(ids, limit), nil
}

func (c *Client) getTopStoriesViaPlaywright(limit int) ([]int, error) {
	if c.playwrightServiceURL == "" {
		return nil, errPlaywrightNotConfigured
	}

	body, err := json.Marshal(map[string]string{"url": topStoriesPageURL})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.playwrightServiceURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d from playwright fetch service: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	htmlBody, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaywrightResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(htmlBody) > maxPlaywrightResponseBytes {
		return nil, fmt.Errorf("playwright response too large (max %d bytes)", maxPlaywrightResponseBytes)
	}

	ids, err := topStoryIDsFromHTML(string(htmlBody))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no top stories found in Playwright response")
	}
	return limitTopStoryIDs(ids, limit), nil
}

func topStoryIDsFromHTML(pageHTML string) ([]int, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}

	var ids []int
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			class, ok := htmlAttr(n, "class")
			if ok && htmlClassContains(class, "athing") {
				if idText, ok := htmlAttr(n, "id"); ok {
					if id, err := strconv.Atoi(idText); err == nil {
						ids = append(ids, id)
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return ids, nil
}

func htmlAttr(n *html.Node, key string) (string, bool) {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}

func htmlClassContains(classList, want string) bool {
	for _, class := range strings.Fields(classList) {
		if class == want {
			return true
		}
	}
	return false
}

func limitTopStoryIDs(ids []int, limit int) []int {
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

// GetItem fetches a single HN item by ID.
func (c *Client) GetItem(id int) (*Item, error) {
	resp, err := c.http.Get(fmt.Sprintf("%s/item/%d.json", baseURL, id))
	if err != nil {
		return nil, fmt.Errorf("fetching item %d: %w", id, err)
	}
	defer resp.Body.Close()

	var item Item
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decoding item %d: %w", id, err)
	}
	return &item, nil
}
