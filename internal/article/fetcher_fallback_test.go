package article

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

var articleLogCaptureMu sync.Mutex

func TestArchivePHFallback_UsesSubmittedSnapshot(t *testing.T) {
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

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available", server.URL+"/wayback/cdx", server.URL+"/")

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

func TestArchivePlaywrightFallback_UsesDaemon(t *testing.T) {
	const renderedText = "playwright rendered article"
	var requestedURL string
	var archiveSubmitCalled bool
	var waybackCalled bool

	playwrightServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode playwright request: %v", err)
		}
		requestedURL = payload.URL
		_, _ = w.Write([]byte("<html><body><main>" + strings.Repeat(renderedText+" ", 40) + "</main></body></html>"))
	}))
	defer playwrightServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/original":
			http.Error(w, "primary failed", http.StatusBadGateway)
		case r.URL.Path == "/archive/submit/":
			archiveSubmitCalled = true
			http.Error(w, "archive should not be reached", http.StatusInternalServerError)
		case r.URL.Path == "/wayback/available":
			waybackCalled = true
			http.Error(w, "wayback should not be reached", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PLAYWRIGHT_FETCH_URL", playwrightServer.URL+"/fetch")
	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available", server.URL+"/wayback/cdx", server.URL+"/")

	fetcher := NewFetcher()
	text, _, err := fetcher.FetchWithTruncation(server.URL + "/original")
	if err != nil {
		t.Fatalf("FetchWithTruncation returned error: %v", err)
	}
	if requestedURL != server.URL+"/original" {
		t.Fatalf("expected playwright fetch for %q, got %q", server.URL+"/original", requestedURL)
	}
	if !strings.Contains(text, "playwright rendered article") {
		t.Fatalf("expected playwright fallback content in text, got %q", text)
	}
	if archiveSubmitCalled {
		t.Fatal("expected archive.ph fallback to be skipped after playwright success")
	}
	if waybackCalled {
		t.Fatal("expected internet archive fallback to be skipped after playwright success")
	}
}

func TestArchiveWaybackFallback_UsesAvailabilitySnapshotURL(t *testing.T) {
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
		case strings.HasPrefix(r.URL.Path, "/web/20240102030405id_/"):
			_, _ = w.Write([]byte("<html><body>" + strings.Repeat(stockLandingText+" ", 30) + "</body></html>"))
		case strings.HasPrefix(r.URL.Path, "/web/20240102030405/"):
			requestedSnapshotPath = r.URL.Path
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available", server.URL+"/wayback/cdx", server.URL+"/")

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
	timestamp, err := time.Parse("20060102150405", requestedAvailabilityTimestamp)
	if err != nil {
		t.Fatalf("expected parseable availability timestamp, got %q: %v", requestedAvailabilityTimestamp, err)
	}
	if delta := time.Since(timestamp.UTC()); delta < -time.Minute || delta > time.Minute {
		t.Fatalf("expected recent availability timestamp, got %q (delta %v)", requestedAvailabilityTimestamp, delta)
	}
	if !strings.HasPrefix(requestedSnapshotPath, "/web/20240102030405/") {
		t.Fatalf("expected wayback snapshot request from availability response, got %q", requestedSnapshotPath)
	}
	if strings.HasPrefix(requestedSnapshotPath, "/web/20240102030405id_/") {
		t.Fatalf("expected wayback snapshot request without id_ modifier, got %q", requestedSnapshotPath)
	}
}

func TestArchiveWaybackFallback_UsesCDXSnapshotWhenAvailabilityMissing(t *testing.T) {
	articleText := fmt.Sprintf("<html><body><main>%s</main></body></html>", strings.Repeat("wayback-cdx ", 80))
	var requestedAvailabilityURL string
	var requestedCDXURL string
	var requestedCDXLimit string
	var requestedCDXSort string
	var requestedSnapshotPath string
	var originalURL string

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
			_, _ = w.Write([]byte(`{"archived_snapshots":{}}`))
		case r.URL.Path == "/wayback/cdx":
			requestedCDXURL = r.URL.Query().Get("url")
			requestedCDXLimit = r.URL.Query().Get("limit")
			requestedCDXSort = r.URL.Query().Get("sort")
			_, _ = fmt.Fprintf(w, `[["timestamp","original"],["20240102030405","%s"]]`, originalURL)
		case strings.HasPrefix(r.URL.Path, "/web/20240102030405/http://"):
			requestedSnapshotPath = r.URL.Path
			_, _ = w.Write([]byte(articleText))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()
	originalURL = server.URL + "/original"

	setTestFallbackPrefixes(t, server.URL+"/archive/", server.URL+"/wayback/available", server.URL+"/wayback/cdx", server.URL+"/")

	fetcher := NewFetcher()
	var text string
	logs := captureArticleLogs(t, func() {
		var err error
		text, _, err = fetcher.FetchWithTruncation(originalURL)
		if err != nil {
			t.Fatalf("FetchWithTruncation returned error: %v", err)
		}
	})
	if !strings.Contains(text, "wayback-cdx") {
		t.Fatalf("expected internet archive CDX fallback content in text, got %q", text)
	}
	if requestedAvailabilityURL != originalURL {
		t.Fatalf("expected availability lookup for %q, got %q", originalURL, requestedAvailabilityURL)
	}
	if requestedCDXURL != originalURL {
		t.Fatalf("expected CDX lookup for %q, got %q", originalURL, requestedCDXURL)
	}
	if requestedCDXLimit != "1" {
		t.Fatalf("expected CDX lookup limit=1, got %q", requestedCDXLimit)
	}
	if requestedCDXSort != "reverse" {
		t.Fatalf("expected CDX lookup sort=reverse, got %q", requestedCDXSort)
	}
	if requestedSnapshotPath != "/web/20240102030405/"+originalURL {
		t.Fatalf("expected CDX replay snapshot path %q, got %q", "/web/20240102030405/"+originalURL, requestedSnapshotPath)
	}
	expectedLogURL := server.URL + "/web/20240102030405/" + originalURL
	if !strings.Contains(logs, "article fetch attempt (internet archive): "+expectedLogURL) {
		t.Fatalf("expected internet archive log to include resolved replay URL, got logs:\n%s", logs)
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

func setTestFallbackPrefixes(t *testing.T, archivePH, waybackAvailability, waybackCDX, waybackReplay string) {
	t.Helper()
	oldArchive := archivePHBaseURL
	oldWaybackAvailability := waybackAvailabilityAPIURL
	oldWaybackCDX := waybackCDXAPIURL
	oldWaybackReplay := waybackReplayBaseURL
	archivePHBaseURL = archivePH
	waybackAvailabilityAPIURL = waybackAvailability
	waybackCDXAPIURL = waybackCDX
	waybackReplayBaseURL = waybackReplay
	t.Cleanup(func() {
		archivePHBaseURL = oldArchive
		waybackAvailabilityAPIURL = oldWaybackAvailability
		waybackCDXAPIURL = oldWaybackCDX
		waybackReplayBaseURL = oldWaybackReplay
	})
}

func captureArticleLogs(t *testing.T, fn func()) string {
	t.Helper()
	articleLogCaptureMu.Lock()
	defer articleLogCaptureMu.Unlock()

	origLogger := articleLogger
	var buf bytes.Buffer
	articleLogger = log.New(&buf, "", 0)
	defer func() {
		articleLogger = origLogger
	}()

	fn()
	return buf.String()
}
