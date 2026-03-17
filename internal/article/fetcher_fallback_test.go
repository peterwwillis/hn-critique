package article

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetchWithTruncation_UsesArchivePHFallback(t *testing.T) {
	articleContent := strings.Repeat("archive-ph ", 40)
	articleText := "<html><body><article>" + articleContent + "</article></body></html>"
	var submitMethod string
	var submitBody url.Values
	var requestedSnapshotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case r.URL.Path == "/archive/submit/":
			submitMethod = r.Method
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read submit body: %v", err)
			}
			submitBody, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse submit body: %v", err)
			}
			w.Header().Set("Refresh", "0;url=/archive/wip/latest")
		case r.URL.Path == "/archive/latest":
			requestedSnapshotPath = r.URL.Path
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available")

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if !strings.Contains(text, "archive-ph") {
		t.Fatalf("expected archive.ph fallback content in text, got %q", text)
	}
	if submitMethod != http.MethodPost {
		t.Fatalf("expected archive.ph submit POST, got %q", submitMethod)
	}
	if got := submitBody.Get("url"); got != server.URL+"/original" {
		t.Fatalf("expected archive.ph submit url %q, got %q", server.URL+"/original", got)
	}
	if got := submitBody.Get("anyway"); got != "1" {
		t.Fatalf("expected archive.ph submit anyway=1, got %q", got)
	}
	if requestedSnapshotPath != "/archive/latest" {
		t.Fatalf("expected normalized archive.ph snapshot path %q, got %q", "/archive/latest", requestedSnapshotPath)
	}
}

func TestFetchWithTruncation_UsesInternetArchiveFallback(t *testing.T) {
	articleText := fmt.Sprintf("<html><body><main>%s</main></body></html>", strings.Repeat("wayback ", 80))
	const stockLandingText = "Please Don't Scroll Past This"
	var requestedAvailabilityURL string
	var requestedAvailabilityTimestamp string
	var requestedSnapshotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case r.URL.Path == "/archive/submit/":
			w.Header().Set("Location", "/archive/not-enough")
			w.WriteHeader(http.StatusFound)
		case r.URL.Path == "/archive/not-enough":
			_, _ = w.Write([]byte("<html><body>too short</body></html>"))
		case r.URL.Path == "/wayback/available":
			requestedAvailabilityURL = r.URL.Query().Get("url")
			requestedAvailabilityTimestamp = r.URL.Query().Get("timestamp")
			fmt.Fprintf(w, `{"archived_snapshots":{"closest":{"available":true,"status":"200","url":"http://%s/web/20240102030405/%s"}}}`, r.Host, requestedAvailabilityURL)
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

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available")

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
	if len(requestedAvailabilityTimestamp) != 14 {
		t.Fatalf("expected 14-digit availability timestamp, got %q", requestedAvailabilityTimestamp)
	}
	if !strings.HasPrefix(requestedSnapshotPath, "/web/20240102030405id_/") {
		t.Fatalf("expected exact replay snapshot request, got %q", requestedSnapshotPath)
	}
}

func TestArchivePHResponseURL_UsesLocationHeader(t *testing.T) {
	base, err := url.Parse("https://archive.ph/submit/")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	headers := http.Header{}
	headers.Set("Location", "/abc123")

	got, ok := archivePHResponseURL(base, headers)
	if !ok {
		t.Fatal("expected archive.ph response URL to be found")
	}
	if got != "https://archive.ph/abc123" {
		t.Fatalf("expected location-based archive.ph url %q, got %q", "https://archive.ph/abc123", got)
	}
}

func setTestFallbackPrefixes(t *testing.T, archivePH, waybackAvailability string) {
	t.Helper()
	oldArchive := archivePHBaseURL
	oldWaybackAvailability := waybackAvailabilityAPIURL
	archivePHBaseURL = archivePH
	waybackAvailabilityAPIURL = waybackAvailability
	t.Cleanup(func() {
		archivePHBaseURL = oldArchive
		waybackAvailabilityAPIURL = oldWaybackAvailability
	})
}
