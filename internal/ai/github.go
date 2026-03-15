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
		http:     newHTTPClient(),
	}
}

func (p *githubProvider) Name() string { return "github" }

func (p *githubProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	prompt := articlePrompt(title, articleURL, content)

	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, p.model, prompt, true)
		if err != nil {
			return nil, fmt.Errorf("github models article analysis: %w", err)
		}

		critique, err := parseArticleCritique(text)
		if err == nil {
			return critique, nil
		}
		if attempt == maxOutputAttempts {
			return nil, fmt.Errorf("github models: invalid article critique output: %w", err)
		}
	}
	return nil, fmt.Errorf("github models: article critique unavailable after retries")
}

func (p *githubProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	if len(comments) == 0 {
		return &generator.CommentsCritique{
			Summary:  "No comments to analyze.",
			Comments: []generator.AnalyzedComment{},
		}, nil
	}
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments))

	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, p.model, prompt, true)
		if err != nil {
			return nil, fmt.Errorf("github models comments analysis: %w", err)
		}

		critique, err := parseCommentsCritique(text, comments)
		if err == nil {
			return critique, nil
		}
		if attempt == maxOutputAttempts {
			return nil, fmt.Errorf("github models: invalid comments critique output: %w", err)
		}
	}
	return nil, fmt.Errorf("github models: comments critique unavailable after retries")
}
