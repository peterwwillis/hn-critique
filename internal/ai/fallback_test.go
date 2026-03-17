package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

var logCaptureMu sync.Mutex

func TestOpenAIFallbackOnRateLimitLogsSuccess(t *testing.T) {
	var calledModels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		calledModels = append(calledModels, req.Model)

		if req.Model == "model-a" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"mainPoints\":[\"p\"],\"truthfulness\":\"t\",\"considerations\":[\"c\"],\"rating\":\"reliable\"}"}}]}`))
	}))
	defer server.Close()

	settings := config.DefaultModelConfig()
	p := newOpenAIProvider(
		config.OpenAIConfig{
			APIKey:    "test-key",
			BaseURL:   server.URL,
			ChatModel: "model-a",
		},
		settings,
		settings,
		[]string{"model-b"},
		[]config.ModelConfig{settings},
	)
	p.http = server.Client()

	logs := captureLogs(t, func() {
		_, err := p.AnalyzeArticle("title", "https://example.com", "content")
		if err != nil {
			t.Fatalf("AnalyzeArticle returned error: %v", err)
		}
	})

	if len(calledModels) != 2 || calledModels[0] != "model-a" || calledModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", calledModels)
	}
	if !strings.Contains(logs, `openai article analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func TestGitHubFallbackOnRateLimitLogsSuccess(t *testing.T) {
	var calledModels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		calledModels = append(calledModels, req.Model)

		if req.Model == "model-a" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"overview\",\"comments\":[{\"id\":1,\"author\":\"alice\",\"text\":\"snippet\",\"indicators\":[\"thoughtful\"],\"accuracyRank\":1,\"analysis\":\"looks good\"}]}"}}]}`))
	}))
	defer server.Close()

	settings := config.DefaultModelConfig()
	p := newGitHubProvider(
		config.GitHubConfig{
			Token:    "gh-token",
			Endpoint: server.URL,
			Model:    "model-a",
		},
		settings,
		[]githubFallback{{model: "model-b", settings: settings}},
	)
	p.http = server.Client()

	comments := []*generator.Comment{{ID: 1, Author: "alice", Text: "hello"}}
	logs := captureLogs(t, func() {
		_, err := p.AnalyzeComments("title", "https://example.com", comments)
		if err != nil {
			t.Fatalf("AnalyzeComments returned error: %v", err)
		}
	})

	if len(calledModels) != 2 || calledModels[0] != "model-a" || calledModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", calledModels)
	}
	if !strings.Contains(logs, `github models comments analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	logCaptureMu.Lock()
	defer logCaptureMu.Unlock()

	origWriter := log.Writer()
	origFlags := log.Flags()
	origPrefix := log.Prefix()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(origWriter)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	}()

	fn()
	return buf.String()
}
