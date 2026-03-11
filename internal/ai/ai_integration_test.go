//go:build integration

package ai_test

import (
	"os"
	"strings"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/config"
)

const aiTestArticleContent = "Go is an open-source programming language created at Google. " +
	"It is statically typed, compiled, and designed for simplicity and performance."

// TestAI_GitHubModels tests AI analysis using the GitHub Models API.
// It requires a GITHUB_TOKEN environment variable and the models:read permission.
// The test is skipped when the token is absent or when the API is inaccessible.
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

	t.Logf("Testing GitHub Models provider (%s)...", cfg.GitHub.Model)

	critique, err := p.AnalyzeArticle(
		"Introduction to Go",
		"https://go.dev",
		aiTestArticleContent,
	)
	if err != nil {
		// HTTP 4xx errors indicate the token lacks models:read permission.
		// Skip rather than fail so that workflows without the permission still pass.
		if strings.Contains(err.Error(), "HTTP 4") {
			t.Skipf("GitHub Models API not accessible (models:read permission may be missing): %v", err)
		}
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
	_, err = p.AnalyzeComments("Introduction to Go", "https://go.dev", nil)
	if err != nil {
		// nil comments might or might not be acceptable; log but don't fail.
		t.Logf("AnalyzeComments (empty): %v", err)
	}
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

	t.Logf("Testing OpenAI provider (%s)...", cfg.OpenAI.ChatModel)

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
