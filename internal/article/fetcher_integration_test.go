//go:build integration

package article_test

import (
	"testing"

	"github.com/peterwwillis/hn-critique/internal/article"
)

// TestIntegration_Fetch tests fetching text from a stable, publicly accessible URL.
// go.dev is the official Go website and returns substantial content.
func TestIntegration_Fetch(t *testing.T) {
	f := article.NewFetcher()
	// Use go.dev — a stable, content-rich page that returns >300 chars directly.
	text, _, err := f.Fetch("https://go.dev")
	if err != nil {
		t.Fatalf("Fetch(go.dev): %v", err)
	}
	if len(text) < 100 {
		t.Errorf("Fetch returned too little text (%d chars)", len(text))
	}
	t.Logf("Fetched %d chars from go.dev", len(text))
}

// TestIntegration_ExtractText verifies HTML extraction on a known HTML string.
func TestIntegration_ExtractText(t *testing.T) {
	html := `<html><body><article><p>Hello integration world</p></article></body></html>`
	got := article.ExtractText(html)
	if got == "" {
		t.Error("ExtractText returned empty string")
	}
	t.Logf("ExtractText: %q", got)
}
