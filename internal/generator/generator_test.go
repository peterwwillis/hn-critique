package generator_test

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
				Rating:         "questionable",
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

	expectedFiles := []string{
		"index.html",
		"style.css",
		".nojekyll",
		filepath.Join("critique", "12345.html"),
		filepath.Join("comments", "12345.html"),
		filepath.Join("critique", "67890.html"),
		filepath.Join("comments", "67890.html"),
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
		"critique/12345.html",
		"comments/12345.html",
		"Questionable",
		"rating-questionable",
		"Ask HN: Favorite tools?",
		"HN Critique",
		"Disclaimer: This website uses AI to generate automated critiques and ratings.",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html missing expected content: %q", want)
		}
	}

	// Verify critique page contains expected content.
	critiqueData, err := os.ReadFile(filepath.Join(outDir, "critique", "12345.html"))
	if err != nil {
		t.Fatalf("reading critique/12345.html: %v", err)
	}
	critique := string(critiqueData)
	for _, want := range []string{
		"Go 1.24 Released",
		"Go 1.24 introduces performance improvements.",
		"Faster GC",
		"questionable",
		"rating-questionable",
		"Disclaimer: This website uses AI to generate automated critiques and ratings.",
	} {
		if !strings.Contains(critique, want) {
			t.Errorf("critique/12345.html missing expected content: %q", want)
		}
	}

	// Verify comments page contains expected content.
	commentsData, err := os.ReadFile(filepath.Join(outDir, "comments", "12345.html"))
	if err != nil {
		t.Fatalf("reading comments/12345.html: %v", err)
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
			t.Errorf("comments/12345.html missing expected content: %q", want)
		}
	}
}
