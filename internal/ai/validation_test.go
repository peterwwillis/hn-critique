package ai

import (
	"html/template"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

func TestValidateArticleCritique(t *testing.T) {
	critique := &generator.ArticleCritique{
		Summary:        "Summary",
		MainPoints:     []string{"Point one"},
		Truthfulness:   "Truthful",
		Considerations: []string{"Consideration"},
		Rating:         "Reliable",
	}

	if err := validateArticleCritique(critique); err != nil {
		t.Fatalf("expected valid critique, got error: %v", err)
	}
	if critique.Rating != "reliable" {
		t.Fatalf("expected normalized rating, got %q", critique.Rating)
	}

	critique.Rating = "needs citation"
	if err := validateArticleCritique(critique); err != nil {
		t.Fatalf("expected valid critique for needs citation, got error: %v", err)
	}
	if critique.Rating != "needs citation" {
		t.Fatalf("expected normalized rating, got %q", critique.Rating)
	}

	critique.Rating = "unknown"
	if err := validateArticleCritique(critique); err == nil {
		t.Fatal("expected error for invalid rating")
	}
}

func TestValidateCommentsCritique(t *testing.T) {
	expected := []*generator.Comment{
		{ID: 101, Author: "a", Text: template.HTML("text")},
		{ID: 102, Author: "b", Text: template.HTML("text")},
	}

	valid := &generator.CommentsCritique{
		Summary: "Summary",
		Comments: []generator.AnalyzedComment{
			{
				ID:           101,
				Author:       "a",
				Text:         "Snippet",
				Indicators:   []string{"thoughtful"},
				AccuracyRank: 1,
				Analysis:     "Analysis",
			},
			{
				ID:           102,
				Author:       "b",
				Text:         "Snippet",
				Indicators:   []string{"trolling"},
				AccuracyRank: 2,
				Analysis:     "Analysis",
			},
		},
	}

	if err := validateCommentsCritique(valid, expected); err != nil {
		t.Fatalf("expected valid comments critique, got error: %v", err)
	}

	invalid := &generator.CommentsCritique{
		Summary: "Summary",
		Comments: []generator.AnalyzedComment{
			{
				ID:           101,
				Author:       "a",
				Text:         "Snippet",
				Indicators:   []string{"unknown"},
				AccuracyRank: 1,
				Analysis:     "Analysis",
			},
			{
				ID:           102,
				Author:       "b",
				Text:         "Snippet",
				Indicators:   []string{"trolling"},
				AccuracyRank: 2,
				Analysis:     "Analysis",
			},
		},
	}

	if err := validateCommentsCritique(invalid, expected); err == nil {
		t.Fatal("expected error for invalid indicator")
	}
}

func TestValidateCommentsCritiqueTruncatesLongText(t *testing.T) {
	expected := []*generator.Comment{
		{ID: 101, Author: "a", Text: template.HTML("original text")},
	}

	// Build a text that is exactly maxCommentSnippetChars+10 runes long.
	longText := ""
	for i := 0; i < maxCommentSnippetChars+10; i++ {
		longText += "x"
	}
	if len([]rune(longText)) <= maxCommentSnippetChars {
		t.Fatalf("test setup error: long text is not long enough")
	}

	c := &generator.CommentsCritique{
		Summary: "Summary",
		Comments: []generator.AnalyzedComment{
			{
				ID:           101,
				Author:       "a",
				Text:         longText,
				Indicators:   []string{"thoughtful"},
				AccuracyRank: 1,
				Analysis:     "Analysis",
			},
		},
	}

	if err := validateCommentsCritique(c, expected); err != nil {
		t.Fatalf("expected truncation, got error: %v", err)
	}
	if got := len([]rune(c.Comments[0].Text)); got != maxCommentSnippetChars {
		t.Errorf("expected text truncated to %d runes, got %d", maxCommentSnippetChars, got)
	}
}
