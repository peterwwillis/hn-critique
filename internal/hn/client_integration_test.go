//go:build integration

package hn_test

import (
	"testing"

	"github.com/peterwwillis/hn-critique/internal/hn"
)

// TestAI prefix is NOT used here — these are non-AI integration tests.

// TestIntegration_GetTopStories fetches a small number of real top-story IDs
// from the Hacker News Firebase API.
func TestIntegration_GetTopStories(t *testing.T) {
	client := hn.NewClient()
	ids, err := client.GetTopStories(5)
	if err != nil {
		t.Fatalf("GetTopStories: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("GetTopStories returned no IDs")
	}
	t.Logf("GetTopStories returned %d IDs (first: %d)", len(ids), ids[0])
}

// TestIntegration_GetItem fetches a known stable HN item.
// Item 1 is the very first HN post and will always exist.
func TestIntegration_GetItem(t *testing.T) {
	client := hn.NewClient()
	item, err := client.GetItem(1)
	if err != nil {
		t.Fatalf("GetItem(1): %v", err)
	}
	if item.ID != 1 {
		t.Errorf("item.ID = %d, want 1", item.ID)
	}
	t.Logf("Item 1: type=%q by=%q", item.Type, item.By)
}

// TestIntegration_GetTopStoriesAndItem fetches the first real top story and
// retrieves its full item data.
func TestIntegration_GetTopStoriesAndItem(t *testing.T) {
	client := hn.NewClient()

	ids, err := client.GetTopStories(1)
	if err != nil {
		t.Fatalf("GetTopStories: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("GetTopStories returned no IDs")
	}

	item, err := client.GetItem(ids[0])
	if err != nil {
		t.Fatalf("GetItem(%d): %v", ids[0], err)
	}
	if item.ID != ids[0] {
		t.Errorf("item.ID = %d, want %d", item.ID, ids[0])
	}
	if item.Type != "story" {
		t.Errorf("item.Type = %q, want \"story\"", item.Type)
	}
	if item.Title == "" {
		t.Error("item.Title is empty")
	}
	t.Logf("Top story #%d: %q (%s)", item.ID, item.Title, item.URL)
}
