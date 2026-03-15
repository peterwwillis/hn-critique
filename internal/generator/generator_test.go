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
				Rating:         "needs citation",
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
			Critique: &generator.ArticleCritique{
				Summary:      "Summary unavailable because the article could not be retrieved.",
				Truthfulness: "Truthfulness assessment unavailable because the article could not be retrieved.",
				Rating:       "unavailable",
			},
		},
		{
			ID:           24680,
			Rank:         3,
			Title:        "Reliable Economic Report",
			URL:          "https://example.com/report",
			Domain:       "example.com",
			Score:        150,
			Author:       "reporter",
			Time:         1741708800,
			CommentCount: 12,
			Critique: &generator.ArticleCritique{
				Summary:        "The report summarizes recent economic indicators.",
				MainPoints:     []string{"GDP growth slowed", "Inflation cooled"},
				Truthfulness:   "The data aligns with published figures.",
				Considerations: []string{"Regional differences were not covered."},
				Rating:         "reliable",
			},
		},
	}

	if err := gen.Generate(stories); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	critiquePath := filepath.FromSlash(stories[0].CritiquePath)
	commentsPath := filepath.FromSlash(stories[0].CommentsPath)
	secondCritiquePath := filepath.FromSlash(stories[1].CritiquePath)
	secondCommentsPath := filepath.FromSlash(stories[1].CommentsPath)
	thirdCritiquePath := filepath.FromSlash(stories[2].CritiquePath)
	thirdCommentsPath := filepath.FromSlash(stories[2].CommentsPath)

	expectedFiles := []string{
		"index.html",
		"style.css",
		".nojekyll",
		critiquePath,
		commentsPath,
		secondCritiquePath,
		secondCommentsPath,
		thirdCritiquePath,
		thirdCommentsPath,
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
		stories[0].CritiquePath,
		stories[0].CommentsPath,
		"Needs Citation",
		"rating-needs-citation",
		"Reliable",
		"rating-reliable",
		"Unavailable",
		"rating-unavailable",
		"Ask HN: Favorite tools?",
		"HN Critique",
		"Disclaimer: This website uses AI to generate automated critiques and ratings.",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html missing expected content: %q", want)
		}
	}

	// Verify critique page contains expected content.
	critiqueData, err := os.ReadFile(filepath.Join(outDir, critiquePath))
	if err != nil {
		t.Fatalf("reading %s: %v", critiquePath, err)
	}
	critique := string(critiqueData)
	for _, want := range []string{
		"Go 1.24 Released",
		"Go 1.24 introduces performance improvements.",
		"Faster GC",
		"Needs Citation",
		"rating-needs-citation",
		"Disclaimer: This website uses AI to generate automated critiques and ratings.",
	} {
		if !strings.Contains(critique, want) {
			t.Errorf("%s missing expected content: %q", critiquePath, want)
		}
	}

	// Verify comments page contains expected content.
	commentsData, err := os.ReadFile(filepath.Join(outDir, commentsPath))
	if err != nil {
		t.Fatalf("reading %s: %v", commentsPath, err)
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
			t.Errorf("%s missing expected content: %q", commentsPath, want)
		}
	}
}
