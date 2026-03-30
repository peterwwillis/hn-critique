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
	var calledModelsMu sync.Mutex
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
		calledModelsMu.Lock()
		calledModels = append(calledModels, req.Model)
		calledModelsMu.Unlock()

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

	calledModelsMu.Lock()
	gotModels := append([]string(nil), calledModels...)
	calledModelsMu.Unlock()
	if len(gotModels) != 2 || gotModels[0] != "model-a" || gotModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", gotModels)
	}
	if !strings.Contains(logs, `openai article analysis: model "model-a" rate-limited; trying next fallback model`) {
		t.Fatalf("missing rate-limit progression log, got logs: %s", logs)
	}
	if !strings.Contains(logs, `openai article analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func TestOpenAICommentsFallbackOnRateLimitLogsSuccess(t *testing.T) {
	var calledModels []string
	var calledModelsMu sync.Mutex
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
		calledModelsMu.Lock()
		calledModels = append(calledModels, req.Model)
		calledModelsMu.Unlock()

		if req.Model == "model-a" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"overview\",\"comments\":[{\"id\":1,\"author\":\"alice\",\"text\":\"snippet\",\"indicators\":[\"thoughtful\"],\"accuracyRank\":1,\"analysis\":\"looks good\"}]}"}}]}`))
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

	comments := []*generator.Comment{{ID: 1, Author: "alice", Text: "hello"}}
	logs := captureLogs(t, func() {
		_, err := p.AnalyzeComments("title", "https://example.com", comments)
		if err != nil {
			t.Fatalf("AnalyzeComments returned error: %v", err)
		}
	})

	calledModelsMu.Lock()
	gotModels := append([]string(nil), calledModels...)
	calledModelsMu.Unlock()
	if len(gotModels) != 2 || gotModels[0] != "model-a" || gotModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", gotModels)
	}
	if !strings.Contains(logs, `openai comments analysis: model "model-a" rate-limited; trying next fallback model`) {
		t.Fatalf("missing rate-limit progression log, got logs: %s", logs)
	}
	if !strings.Contains(logs, `openai comments analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func TestGitHubArticleFallbackOnRateLimitLogsSuccess(t *testing.T) {
	var calledModels []string
	var calledModelsMu sync.Mutex
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
		calledModelsMu.Lock()
		calledModels = append(calledModels, req.Model)
		calledModelsMu.Unlock()

		if req.Model == "model-a" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"mainPoints\":[\"p\"],\"truthfulness\":\"t\",\"considerations\":[\"c\"],\"rating\":\"reliable\"}"}}]}`))
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

	logs := captureLogs(t, func() {
		_, err := p.AnalyzeArticle("title", "https://example.com", "content")
		if err != nil {
			t.Fatalf("AnalyzeArticle returned error: %v", err)
		}
	})

	calledModelsMu.Lock()
	gotModels := append([]string(nil), calledModels...)
	calledModelsMu.Unlock()
	if len(gotModels) != 2 || gotModels[0] != "model-a" || gotModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", gotModels)
	}
	if !strings.Contains(logs, `github models article analysis: model "model-a" rate-limited; trying next fallback model`) {
		t.Fatalf("missing rate-limit progression log, got logs: %s", logs)
	}
	if !strings.Contains(logs, `github models article analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func TestGitHubArticleRetryIncludesMissingFieldFeedback(t *testing.T) {
	var prompts []string
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
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if len(req.Messages) < 2 {
			t.Fatalf("messages = %d, want >= 2", len(req.Messages))
		}
		prompts = append(prompts, req.Messages[1].Content)
		if len(prompts) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"truthfulness\":\"t\",\"considerations\":[\"c\"],\"rating\":\"reliable\"}"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"mainPoints\":[\"p\"],\"truthfulness\":\"t\",\"considerations\":[\"c\"],\"rating\":\"reliable\"}"}}]}`))
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
		nil,
	)
	p.http = server.Client()

	_, err := p.AnalyzeArticle("title", "https://example.com", "content")
	if err != nil {
		t.Fatalf("AnalyzeArticle returned error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("request count = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], "mainPoints is required") {
		t.Fatalf("retry prompt missing validation feedback: %q", prompts[1])
	}
	if !strings.Contains(prompts[1], `"mainPoints": ["Point one", "Point two"]`) {
		t.Fatalf("retry prompt missing one-shot example: %q", prompts[1])
	}
}

func TestGitHubFallbackOnRateLimitLogsSuccess(t *testing.T) {
	var calledModels []string
	var calledModelsMu sync.Mutex
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
		calledModelsMu.Lock()
		calledModels = append(calledModels, req.Model)
		calledModelsMu.Unlock()

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

	calledModelsMu.Lock()
	gotModels := append([]string(nil), calledModels...)
	calledModelsMu.Unlock()
	if len(gotModels) != 2 || gotModels[0] != "model-a" || gotModels[1] != "model-b" {
		t.Fatalf("called models = %v, want [model-a model-b]", gotModels)
	}
	if !strings.Contains(logs, `github models comments analysis: model "model-a" rate-limited; trying next fallback model`) {
		t.Fatalf("missing rate-limit progression log, got logs: %s", logs)
	}
	if !strings.Contains(logs, `github models comments analysis: fallback model "model-b" succeeded after earlier rate-limit`) {
		t.Fatalf("missing fallback success log, got logs: %s", logs)
	}
}

func TestOpenAIArticleRetryIncludesMissingFieldFeedback(t *testing.T) {
	var prompts []string
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
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if len(req.Messages) < 2 {
			t.Fatalf("messages = %d, want >= 2", len(req.Messages))
		}
		prompts = append(prompts, req.Messages[1].Content)
		if len(prompts) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"s\",\"truthfulness\":\"t\",\"considerations\":[\"c\"],\"rating\":\"reliable\"}"}}]}`))
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
		nil,
		nil,
	)
	p.http = server.Client()

	_, err := p.AnalyzeArticle("title", "https://example.com", "content")
	if err != nil {
		t.Fatalf("AnalyzeArticle returned error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("request count = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], "mainPoints is required") {
		t.Fatalf("retry prompt missing validation feedback: %q", prompts[1])
	}
	if !strings.Contains(prompts[1], `"mainPoints": ["Point one", "Point two"]`) {
		t.Fatalf("retry prompt missing one-shot example: %q", prompts[1])
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
