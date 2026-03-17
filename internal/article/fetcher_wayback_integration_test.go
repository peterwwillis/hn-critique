//go:build integration

package article

import (
	"strings"
	"testing"
)

func TestIntegration_InternetArchiveSnapshotURL_Live(t *testing.T) {
	f := NewFetcher()

	snapshotURL, err := f.internetArchiveSnapshotURL("https://kagi.com/smallweb/")
	if err != nil {
		t.Fatalf("internetArchiveSnapshotURL(kagi.com/smallweb): %v", err)
	}
	if !strings.HasPrefix(snapshotURL, "https://web.archive.org/web/") {
		t.Fatalf("expected Wayback replay URL, got %q", snapshotURL)
	}
	if !strings.Contains(snapshotURL, "kagi.com/smallweb") {
		t.Fatalf("expected Wayback replay URL to include original URL, got %q", snapshotURL)
	}

	text, _, err := f.fetchURL(snapshotURL)
	if err != nil {
		t.Fatalf("fetchURL(%q): %v", snapshotURL, err)
	}
	if len(text) < 300 {
		t.Fatalf("expected substantial archived content, got %d chars from %q", len(text), snapshotURL)
	}
	if !strings.Contains(strings.ToLower(text), "small web") {
		t.Fatalf("expected archived article text to mention %q, got %q", "small web", text[:min(len(text), 200)])
	}
}
