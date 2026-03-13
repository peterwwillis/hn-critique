//go:build integration

package ai_test

import (
	"os"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/config"
)

const aiTestArticleContent = "Go is an open-source programming language created at Google. " +
	"It is statically typed, compiled, and designed for simplicity and performance."

// TestAI_GitHubModels tests AI analysis using the GitHub Models API.
//
// The test requires GITHUB_TOKEN to be set in the environment.  In GitHub
// Actions this token is injected automatically; the workflow job that runs
// this test grants `permissions: models: read` so that the token is
// authorised to call the inference endpoint.
//
// The test is skipped only when GITHUB_TOKEN is explicitly absent (i.e. a
// local run where no token was provided).  When the token IS present, any
// API failure is treated as a real test failure — never a silent skip — so
// that the caller can see exactly what went wrong.
func TestAI_GitHubModels(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set; skipping GitHub Models AI test")
	}

	cfg := config.Defaults()
	cfg.Provider = config.ProviderGitHub
	cfg.GitHub.Token = token

	p, err := ai.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	t.Logf("Testing GitHub Models provider (model: %s, endpoint: %s)...", cfg.GitHub.Model, cfg.GitHub.Endpoint)

	critique, err := p.AnalyzeArticle(
		"Introduction to Go",
		"https://go.dev",
		aiTestArticleContent,
	)
	if err != nil {
		t.Fatalf("AnalyzeArticle: %v", err)
	}

	if critique == nil {
		t.Fatal("AnalyzeArticle returned nil critique")
	}
	if critique.Summary == "" {
		t.Error("critique.Summary is empty")
	}
	if len(critique.MainPoints) == 0 {
		t.Error("critique.MainPoints is empty")
	}
	t.Logf("GitHub Models critique — rating: %q, summary: %q", critique.Rating, critique.Summary)

	// Also test comment analysis.
	t.Logf("Testing comments analysis with GitHub Models...")
	commentsCritique, err := p.AnalyzeComments("Introduction to Go", "https://go.dev", nil)
	if err != nil {
		t.Fatalf("AnalyzeComments: %v", err)
	}
	if commentsCritique == nil {
		t.Fatal("AnalyzeComments returned nil")
	}
	t.Logf("Comments critique — summary: %q", commentsCritique.Summary)
}

// TestAI_OpenAI tests AI analysis using the OpenAI API.
// It is skipped when OPENAI_API_KEY is not set.
func TestAI_OpenAI(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set; skipping OpenAI AI test")
	}

	cfg := config.Defaults()
	cfg.Provider = config.ProviderOpenAI
	cfg.OpenAI.APIKey = apiKey

	p, err := ai.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	t.Logf("Testing OpenAI provider (model: %s)...", cfg.OpenAI.ChatModel)

	critique, err := p.AnalyzeArticle(
		"Introduction to Go",
		"https://go.dev",
		aiTestArticleContent,
	)
	if err != nil {
		t.Fatalf("AnalyzeArticle: %v", err)
	}
	if critique == nil {
		t.Fatal("AnalyzeArticle returned nil critique")
	}
	if critique.Summary == "" {
		t.Error("critique.Summary is empty")
	}
	t.Logf("OpenAI critique — rating: %q, summary: %q", critique.Rating, critique.Summary)
}
