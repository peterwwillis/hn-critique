package ai

import (
	"html/template"
	"strings"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

func TestBuildCommentTextKeepsFullComment(t *testing.T) {
	longText := strings.Repeat("a", maxCommentChars+100)
	comments := []*generator.Comment{
		{ID: 1, Author: "alice", Text: template.HTML(longText)},
	}

	result := buildCommentText(comments)
	if !strings.Contains(result, longText) {
		t.Fatalf("expected full comment text to be included, length=%d", len(result))
	}
}

func TestApplyCommentTextUsesOriginalHTML(t *testing.T) {
	critique := &generator.CommentsCritique{
		Summary: "summary",
		Comments: []generator.AnalyzedComment{
			{ID: 42, Author: "alice", Text: "snippet", AccuracyRank: 1},
		},
	}
	comments := []*generator.Comment{
		{ID: 42, Author: "alice", Text: template.HTML("<p>Full comment.</p>")},
	}

	applyCommentText(critique, comments)

	if critique.Comments[0].Text != "<p>Full comment.</p>" {
		t.Fatalf("expected comment text to use original HTML, got %q", critique.Comments[0].Text)
	}
}

func TestApplyCommentTextMissingID(t *testing.T) {
	critique := &generator.CommentsCritique{
		Summary: "summary",
		Comments: []generator.AnalyzedComment{
			{ID: 99, Author: "unknown", Text: "original", AccuracyRank: 1},
		},
	}
	comments := []*generator.Comment{
		{ID: 42, Author: "alice", Text: template.HTML("<p>Full comment.</p>")},
	}

	applyCommentText(critique, comments)

	if critique.Comments[0].Text != "original" {
		t.Fatalf("expected comment text to remain unchanged, got %q", critique.Comments[0].Text)
	}
}
