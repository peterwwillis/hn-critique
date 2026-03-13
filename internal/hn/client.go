// Package hn provides a client for the Hacker News Firebase API.
package hn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://hacker-news.firebaseio.com/v0"

// Client is a Hacker News API client.
type Client struct {
	http *http.Client
}

// NewClient returns a new HN API client.
func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
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
	resp, err := c.http.Get(baseURL + "/topstories.json")
	if err != nil {
		return nil, fmt.Errorf("fetching top stories: %w", err)
	}
	defer resp.Body.Close()

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decoding top stories: %w", err)
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
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
