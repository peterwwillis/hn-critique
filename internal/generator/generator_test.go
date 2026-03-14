package generator_test

import (
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

func TestGenerate(t *testing.T) {
	outDir := t.TempDir()

	gen := generator.New(outDir)

	stories := []*generator.Story{
		{
			ID:           12345,
			Rank:         1,
			Title:        "Go 1.24 Released",
			URL:          "https://go.dev/blog/go1.24",
			Domain:       "go.dev",
			Score:        500,
			Author:       "gopher",
			Time:         1741723200,
			CommentCount: 150,
			Comments: []*generator.Comment{
				{
					ID:     99001,
					Author: "alice",
					Text:   template.HTML("<p>Great release!</p>"),
					Time:   1741726800,
					Depth:  0,
				},
			},
			Critique: &generator.ArticleCritique{
				Summary:        "Go 1.24 introduces performance improvements.",
				MainPoints:     []string{"Faster GC", "New crypto packages"},
				Truthfulness:   "Claims appear accurate.",
				Considerations: []string{"Performance varies by workload."},
				Rating:         "reliable",
			},
			CommentsCritique: &generator.CommentsCritique{
				Summary: "Discussion is mostly positive.",
				Comments: []generator.AnalyzedComment{
					{
						ID:           99001,
						Author:       "alice",
						Text:         "Great release!",
						Indicators:   []string{"thoughtful", "constructive"},
						AccuracyRank: 1,
						Analysis:     "Accurate and supportive.",
					},
				},
			},
		},
		{
			ID:           67890,
			Rank:         2,
			Title:        "Ask HN: Favorite tools?",
			URL:          "",
			Domain:       "",
			Score:        200,
			Author:       "asker",
			Time:         1741716000,
			CommentCount: 89,
		},
	}

	if err := gen.Generate(stories); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	firstDatePath := time.Unix(stories[0].Time, 0).UTC().Format("2006/01/02")
	secondDatePath := time.Unix(stories[1].Time, 0).UTC().Format("2006/01/02")
	critiqueFirst := filepath.Join("critique", filepath.FromSlash(firstDatePath), "12345.html")
	commentsFirst := filepath.Join("comments", filepath.FromSlash(firstDatePath), "12345.html")
	critiqueSecond := filepath.Join("critique", filepath.FromSlash(secondDatePath), "67890.html")
	commentsSecond := filepath.Join("comments", filepath.FromSlash(secondDatePath), "67890.html")

	expectedFiles := []string{
		"index.html",
		"style.css",
		".nojekyll",
		critiqueFirst,
		commentsFirst,
		critiqueSecond,
		commentsSecond,
	}
	for _, rel := range expectedFiles {
		full := filepath.Join(outDir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected file missing: %s (%v)", rel, err)
		}
	}

	// Verify index.html contains expected content.
	indexData, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	index := string(indexData)
	for _, want := range []string{
		"Go 1.24 Released",
		"go.dev",
		"500 points",
		"gopher",
		path.Join("critique", firstDatePath, "12345.html"),
		path.Join("comments", firstDatePath, "12345.html"),
		"Ask HN: Favorite tools?",
		"HN Critique",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html missing expected content: %q", want)
		}
	}

	// Verify critique page contains expected content.
	critiqueData, err := os.ReadFile(filepath.Join(outDir, critiqueFirst))
	if err != nil {
		t.Fatalf("reading %s: %v", critiqueFirst, err)
	}
	critique := string(critiqueData)
	for _, want := range []string{
		"Go 1.24 Released",
		"Go 1.24 introduces performance improvements.",
		"Faster GC",
		"reliable",
		"rating-reliable",
	} {
		if !strings.Contains(critique, want) {
			t.Errorf("%s missing expected content: %q", critiqueFirst, want)
		}
	}

	// Verify comments page contains expected content.
	commentsData, err := os.ReadFile(filepath.Join(outDir, commentsFirst))
	if err != nil {
		t.Fatalf("reading %s: %v", commentsFirst, err)
	}
	comments := string(commentsData)
	for _, want := range []string{
		"Discussion is mostly positive.",
		"thoughtful",
		"constructive",
		"Accurate and supportive.",
		"#1",
	} {
		if !strings.Contains(comments, want) {
			t.Errorf("%s missing expected content: %q", commentsFirst, want)
		}
	}
}
