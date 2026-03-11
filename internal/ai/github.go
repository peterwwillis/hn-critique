package ai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

// githubProvider calls the GitHub Models inference API, which is
// OpenAI-compatible and accessible from GitHub Actions using GITHUB_TOKEN
// (requires `permissions: models: read` in the workflow job).
//
// Endpoint: https://models.github.ai/inference/chat/completions
// Docs: https://docs.github.com/en/github-models
type githubProvider struct {
	endpoint string // full URL to the chat/completions endpoint
	token    string
	model    string
	http     *http.Client
}

func newGitHubProvider(cfg config.GitHubConfig) *githubProvider {
	base := strings.TrimRight(cfg.Endpoint, "/")
	return &githubProvider{
		endpoint: base + "/chat/completions",
		token:    cfg.Token,
		model:    cfg.Model,
		http:     &http.Client{Timeout: httpTimeout},
	}
}

func (p *githubProvider) Name() string { return "github" }

func (p *githubProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	prompt := articlePrompt(title, articleURL, content)

	text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, p.model, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("github models article analysis: %w", err)
	}

	var critique generator.ArticleCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("github models: parsing article critique: %w", err)
	}
	critique.Rating = sanitizeRating(critique.Rating)
	return &critique, nil
}

func (p *githubProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments))

	text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, p.model, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("github models comments analysis: %w", err)
	}

	var critique generator.CommentsCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("github models: parsing comments critique: %w", err)
	}
	return &critique, nil
}
