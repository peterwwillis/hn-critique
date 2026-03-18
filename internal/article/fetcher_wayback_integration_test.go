//go:build integration

package article

import (
	"strings"
	"testing"
	"time"
)

func TestIntegration_InternetArchiveSnapshotURL_Live(t *testing.T) {
	f := NewFetcher()

	var (
		snapshotURL string
		text        string
		err         error
	)

	// Resolve the Internet Archive snapshot URL with a small retry/backoff to
	// reduce flakiness from transient network or IA issues.
	for attempt := 0; attempt < 3; attempt++ {
		snapshotURL, err = f.internetArchiveSnapshotURL("https://kagi.com/smallweb/")
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		t.Fatalf("internetArchiveSnapshotURL(kagi.com/smallweb): %v", err)
	}
	if !strings.HasPrefix(snapshotURL, "https://web.archive.org/web/") {
		t.Fatalf("expected Wayback replay URL, got %q", snapshotURL)
	}
	if !strings.Contains(snapshotURL, "kagi.com/smallweb") {
		t.Fatalf("expected Wayback replay URL to include original URL, got %q", snapshotURL)
	}

	// Fetch the archived content with a small retry/backoff as well.
	for attempt := 0; attempt < 3; attempt++ {
		text, _, err = f.fetchURL(snapshotURL)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		t.Fatalf("fetchURL(%q): %v", snapshotURL, err)
	}
	if len(text) < 300 {
		t.Fatalf("expected substantial archived content, got %d chars from %q", len(text), snapshotURL)
	}

	// Avoid depending on the specific phrase "small web"; instead, check for a
	// more stable marker that still indicates we fetched a Kagi-related page.
	if !strings.Contains(strings.ToLower(text), "kagi") {
		t.Fatalf("expected archived article text to mention %q, got %q", "kagi", text[:min(len(text), 200)])
	}
}
