package article

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchWithTruncation_UsesArchivePHFallback(t *testing.T) {
	articleContent := strings.Repeat("archive-ph ", 40)
	articleText := "<html><body><article>" + articleContent + "</article></body></html>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, "/archive/"):
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/")

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if !strings.Contains(text, "archive-ph") {
		t.Fatalf("expected archive.ph fallback content in text, got %q", text)
	}
}

func TestFetchWithTruncation_UsesInternetArchiveFallback(t *testing.T) {
	articleText := fmt.Sprintf("<html><body><main>%s</main></body></html>", strings.Repeat("wayback ", 80))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, "/archive/"):
			_, _ = w.Write([]byte("<html><body>too short</body></html>"))
		case strings.HasPrefix(r.URL.Path, "/wayback/"):
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/")

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if !strings.Contains(text, "wayback") {
		t.Fatalf("expected internet archive fallback content in text, got %q", text)
	}
}

func setTestFallbackPrefixes(t *testing.T, archivePH, wayback string) {
	t.Helper()
	oldArchive := archivePHPrefix
	oldWayback := waybackPrefix
	archivePHPrefix = archivePH
	waybackPrefix = wayback
	t.Cleanup(func() {
		archivePHPrefix = oldArchive
		waybackPrefix = oldWayback
	})
}
