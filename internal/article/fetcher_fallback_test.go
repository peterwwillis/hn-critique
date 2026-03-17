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

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available?url=")

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
	const stockLandingText = "Please Don't Scroll Past This"
	var requestedAvailabilityURL string
	var requestedSnapshotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case strings.HasPrefix(r.URL.Path, "/archive/"):
			_, _ = w.Write([]byte("<html><body>too short</body></html>"))
		case r.URL.Path == "/wayback/available":
			requestedAvailabilityURL = r.URL.Query().Get("url")
			fmt.Fprintf(w, `{"archived_snapshots":{"closest":{"available":true,"url":"http://%s/web/20240102030405/%s"}}}`, r.Host, requestedAvailabilityURL)
		case strings.HasPrefix(r.URL.Path, "/web/20240102030405/"):
			_, _ = w.Write([]byte("<html><body>" + strings.Repeat(stockLandingText+" ", 30) + "</body></html>"))
		case strings.HasPrefix(r.URL.Path, "/web/20240102030405id_/"):
			requestedSnapshotPath = r.URL.Path
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available?url=")

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if !strings.Contains(text, "wayback") {
		t.Fatalf("expected internet archive fallback content in text, got %q", text)
	}
	if strings.Contains(text, stockLandingText) {
		t.Fatalf("expected archived article text, got wayback landing page text %q", text)
	}
	if requestedAvailabilityURL != server.URL+"/original" {
		t.Fatalf("expected availability lookup for %q, got %q", server.URL+"/original", requestedAvailabilityURL)
	}
	if !strings.HasPrefix(requestedSnapshotPath, "/web/20240102030405id_/") {
		t.Fatalf("expected exact replay snapshot request, got %q", requestedSnapshotPath)
	}
}

func setTestFallbackPrefixes(t *testing.T, archivePH, waybackAvailability string) {
	t.Helper()
	oldArchive := archivePHPrefix
	oldWaybackAvailability := waybackAvailabilityPrefix
	archivePHPrefix = archivePH
	waybackAvailabilityPrefix = waybackAvailability
	t.Cleanup(func() {
		archivePHPrefix = oldArchive
		waybackAvailabilityPrefix = oldWaybackAvailability
	})
}
