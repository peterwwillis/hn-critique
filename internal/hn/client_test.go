package hn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTopStories_RetriesDirectAPI(t *testing.T) {
	t.Cleanup(func() {
		baseURL = "https://hacker-news.firebaseio.com/v0"
	})

	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/topstories.json" {
			http.NotFound(w, r)
			return
		}
		attempts++
		if attempts < topStoriesFetchAttempts {
			http.Error(w, "try again", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode([]int{101, 102, 103})
	}))
	defer server.Close()

	baseURL = server.URL + "/v0"

	client := NewClient()
	ids, err := client.GetTopStories(2)
	if err != nil {
		t.Fatalf("GetTopStories returned error: %v", err)
	}
	if attempts != topStoriesFetchAttempts {
		t.Fatalf("expected %d attempts, got %d", topStoriesFetchAttempts, attempts)
	}
	if got, want := len(ids), 2; got != want {
		t.Fatalf("expected %d ids, got %d", want, got)
	}
	if ids[0] != 101 || ids[1] != 102 {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestGetTopStories_FallsBackToPlaywright(t *testing.T) {
	t.Cleanup(func() {
		baseURL = "https://hacker-news.firebaseio.com/v0"
		topStoriesPageURL = topStoriesPageURLDefault
	})

	var directAttempts int
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/topstories.json" {
			http.NotFound(w, r)
			return
		}
		directAttempts++
		http.Error(w, "upstream error", http.StatusBadGateway)
	}))
	defer apiServer.Close()

	var requestedURL string
	playwrightServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		requestedURL = payload.URL
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<html><body>
				<table>
					<tr class="athing submission" id="201"></tr>
					<tr class="athing submission" id="202"></tr>
					<tr class="athing submission" id="203"></tr>
				</table>
			</body></html>`))
	}))
	defer playwrightServer.Close()

	baseURL = apiServer.URL + "/v0"
	topStoriesPageURL = "https://news.ycombinator.com/news"

	client := NewClient()
	client.playwrightServiceURL = playwrightServer.URL

	ids, err := client.GetTopStories(2)
	if err != nil {
		t.Fatalf("GetTopStories returned error: %v", err)
	}
	if directAttempts != topStoriesFetchAttempts {
		t.Fatalf("expected %d direct attempts, got %d", topStoriesFetchAttempts, directAttempts)
	}
	if requestedURL != topStoriesPageURL {
		t.Fatalf("expected playwright to fetch %q, got %q", topStoriesPageURL, requestedURL)
	}
	if got, want := len(ids), 2; got != want {
		t.Fatalf("expected %d ids, got %d", want, got)
	}
	if ids[0] != 201 || ids[1] != 202 {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
