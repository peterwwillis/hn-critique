package article

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchWithTruncation_UsesArchivePHFallback(t *testing.T) {
	const body = "<html><body><article>" + "archive-ph " + "</article></body></html>"
	articleText := strings.Repeat(body, 40)

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

	reset := setTestFallbackPrefixes(server.URL+"/archive/", server.URL+"/wayback/")
	defer reset()

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

	reset := setTestFallbackPrefixes(server.URL+"/archive/", server.URL+"/wayback/")
	defer reset()

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if !strings.Contains(text, "wayback") {
		t.Fatalf("expected internet archive fallback content in text, got %q", text)
	}
}

func setTestFallbackPrefixes(archivePH, wayback string) func() {
	oldArchive := archivePHPrefix
	oldWayback := waybackPrefix
	archivePHPrefix = archivePH
	waybackPrefix = wayback
	return func() {
		archivePHPrefix = oldArchive
		waybackPrefix = oldWayback
	}
}
