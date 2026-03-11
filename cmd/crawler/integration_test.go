//go:build integration

// Package main_test contains end-to-end integration tests for the crawler
// pipeline that exercise the full non-AI path: HN API -> article fetcher ->
// static site generator.
package main_test

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peterwwillis/hn-critique/internal/article"
	"github.com/peterwwillis/hn-critique/internal/generator"
	"github.com/peterwwillis/hn-critique/internal/hn"
)

// TestIntegration_Pipeline runs the full HN->article->generator pipeline for
// a small number of real stories (no AI step).
func TestIntegration_Pipeline(t *testing.T) {
	const storyCount = 3

	hnClient := hn.NewClient()
	articleFetcher := article.NewFetcher()
	outDir := t.TempDir()
	gen := generator.New(outDir)

	t.Logf("Fetching top %d stories from HN...", storyCount)
	ids, err := hnClient.GetTopStories(storyCount)
	if err != nil {
		t.Fatalf("GetTopStories: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("GetTopStories returned no IDs")
	}

	var stories []*generator.Story

	for rank, id := range ids {
		item, err := hnClient.GetItem(id)
		if err != nil || item == nil {
			t.Logf("  skip item %d: %v", id, err)
			continue
		}
		if item.Deleted || item.Dead || item.Type != "story" {
			t.Logf("  skip item %d: deleted/dead/wrong type", id)
			continue
		}

		story := &generator.Story{
			ID:           item.ID,
			Rank:         rank + 1,
			Title:        item.Title,
			URL:          item.URL,
			Domain:       pipelineDomain(item.URL),
			Score:        item.Score,
			Author:       item.By,
			Time:         item.Time,
			CommentCount: item.Descendants,
		}

		// Fetch a few top-level comments.
		topKids := item.Kids
		if len(topKids) > 3 {
			topKids = topKids[:3]
		}
		for _, kid := range topKids {
			c, err := hnClient.GetItem(kid)
			if err != nil || c == nil || c.Deleted || c.Dead || c.Type != "comment" {
				continue
			}
			story.Comments = append(story.Comments, &generator.Comment{
				ID:     c.ID,
				Author: c.By,
				Text:   template.HTML(c.Text), //nolint:gosec // HN sanitizes comment HTML
				Time:   c.Time,
				Depth:  0,
			})
			time.Sleep(50 * time.Millisecond)
		}

		// Attempt to fetch article text (best-effort; skip on failure).
		if item.URL != "" {
			text, err := articleFetcher.Fetch(item.URL)
			if err != nil {
				t.Logf("  article fetch failed for %s: %v (continuing without article text)", item.URL, err)
			} else {
				story.ArticleText = text
				t.Logf("  fetched %d chars from %s", len(text), item.URL)
			}
		}

		stories = append(stories, story)
		time.Sleep(100 * time.Millisecond)
	}

	if len(stories) == 0 {
		t.Fatal("no stories were successfully fetched")
	}

	t.Logf("Generating site for %d stories in %s...", len(stories), outDir)
	if err := gen.Generate(stories); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify top-level output files exist.
	for _, f := range []string{"index.html", "style.css", ".nojekyll"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("expected output file missing: %s (%v)", f, err)
		}
	}

	// Verify index.html contains meaningful content.
	indexBytes, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	index := string(indexBytes)
	if !strings.Contains(index, "HN Critique") {
		t.Error("index.html does not contain 'HN Critique'")
	}
	// Check for the first story's ID rather than raw title text — the title
	// may contain HTML-escaped characters in the output file.
	firstStoryIDStr := fmt.Sprintf("/critique/%d.html", stories[0].ID)
	if !strings.Contains(index, firstStoryIDStr) {
		t.Errorf("index.html does not contain link to first story %s", firstStoryIDStr)
	}

	// Verify per-story pages exist.
	for _, s := range stories {
		for _, sub := range []string{"critique", "comments"} {
			p := filepath.Join(outDir, sub, fmt.Sprintf("%d.html", s.ID))
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s page missing for story %d: %v", sub, s.ID, err)
			}
		}
	}

	t.Logf("Pipeline test passed: %d stories -> site in %s", len(stories), outDir)
}

// pipelineDomain returns the bare hostname from a URL.
func pipelineDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(s, "www.")
}
