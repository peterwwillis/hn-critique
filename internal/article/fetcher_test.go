package article_test

import (
	"strings"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/article"
)

func TestNewFetcher(t *testing.T) {
	f := article.NewFetcher()
	if f == nil {
		t.Error("NewFetcher returned nil")
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		contains    string
		notContains string
	}{
		{
			name:     "simple paragraph",
			html:     `<html><body><p>Hello world</p></body></html>`,
			contains: "Hello world",
		},
		{
			name:        "article tag preferred over nav",
			html:        `<html><body><nav>skip this nav</nav><article><p>Article text here</p></article></body></html>`,
			contains:    "Article text here",
			notContains: "skip this nav",
		},
		{
			name:        "script tags stripped",
			html:        `<html><body><script>alert(1)</script><p>Real content</p></body></html>`,
			contains:    "Real content",
			notContains: "alert(1)",
		},
		{
			name:        "style tags stripped",
			html:        `<html><head><style>body{color:red}</style></head><body><p>Visible text</p></body></html>`,
			contains:    "Visible text",
			notContains: "color:red",
		},
		{
			name:     "plain text passthrough",
			html:     "Just some plain text with no HTML tags",
			contains: "Just some plain text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := article.ExtractText(tc.html)
			if tc.contains != "" && !strings.Contains(got, tc.contains) {
				t.Errorf("ExtractText output missing %q\ngot: %q", tc.contains, got)
			}
			if tc.notContains != "" && strings.Contains(got, tc.notContains) {
				t.Errorf("ExtractText output should not contain %q\ngot: %q", tc.notContains, got)
			}
		})
	}
}
