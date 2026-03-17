package ai

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

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
	settings config.ModelConfig
	// fallbacks are tried in order when the primary model returns HTTP 429.
	fallbacks []githubFallback
	mode      config.ModelMode
	counter   atomic.Uint64
	http      *http.Client
}

// githubFallback holds a model name and its settings to use when the primary
// model is rate-limited.
type githubFallback struct {
	model    string
	settings config.ModelConfig
}

func newGitHubProvider(cfg config.GitHubConfig, settings config.ModelConfig, fallbacks []githubFallback) *githubProvider {
	base := strings.TrimRight(cfg.Endpoint, "/")
	return &githubProvider{
		endpoint:  base + "/chat/completions",
		token:     cfg.Token,
		model:     cfg.Model,
		settings:  settings,
		fallbacks: fallbacks,
		mode:      cfg.ModelMode,
		http:      newHTTPClient(),
	}
}

func (p *githubProvider) Name() string { return "github" }

// allModels returns the primary model followed by all fallback models.
func (p *githubProvider) allModels() []githubFallback {
	models := make([]githubFallback, 0, 1+len(p.fallbacks))
	models = append(models, githubFallback{model: p.model, settings: p.settings})
	models = append(models, p.fallbacks...)
	return models
}

// pickModel returns the single model to use for the current request in
// round-robin mode, cycling through all configured models atomically.
func (p *githubProvider) pickModel() githubFallback {
	all := p.allModels()
	idx := p.counter.Add(1) - 1
	return all[idx%uint64(len(all))]
}

// tryArticleModel attempts article analysis with a specific model.
// It returns ErrRateLimit when the model is rate-limited.
func (p *githubProvider) tryArticleModel(model string, settings config.ModelConfig, title, articleURL, content string) (*generator.ArticleCritique, error) {
	prompt := articlePrompt(title, articleURL, content, settings.Limits.ArticlePromptBytes)
	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, model, prompt, true, settings.Inference)
		if err != nil {
			return nil, err
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

func (p *githubProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	if p.mode == config.ModelModeRoundRobin {
		m := p.pickModel()
		critique, err := p.tryArticleModel(m.model, m.settings, title, articleURL, content)
		if err != nil {
			return nil, fmt.Errorf("github models article analysis: %w", err)
		}
		return critique, nil
	}

	// Default: fallback mode — try each model in order, moving on for HTTP 429.
	var lastRateLimitErr error
	for i, m := range p.allModels() {
		critique, err := p.tryArticleModel(m.model, m.settings, title, articleURL, content)
		if err == nil {
			if i > 0 && lastRateLimitErr != nil {
				log.Printf("github models article analysis: fallback model %q succeeded after earlier rate-limit", m.model)
			}
			return critique, nil
		}
		var rateLimitErr *ErrRateLimit
		if errors.As(err, &rateLimitErr) {
			log.Printf("github models article analysis: model %q rate-limited; trying next fallback model", m.model)
			lastRateLimitErr = err
			continue // try next fallback model
		}
		return nil, fmt.Errorf("github models article analysis: %w", err)
	}
	return nil, fmt.Errorf("github models article analysis: all models rate limited: %w", lastRateLimitErr)
}

// tryCommentsModel attempts comments analysis with a specific model.
// It returns ErrRateLimit when the model is rate-limited.
func (p *githubProvider) tryCommentsModel(model string, settings config.ModelConfig, title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments, settings.Limits.CommentPromptBytes))
	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		text, err := callChatCompletions(p.http, p.endpoint, "Bearer "+p.token, model, prompt, true, settings.Inference)
		if err != nil {
			return nil, err
		}
		critique, err := parseCommentsCritique(text, comments)
		if err == nil {
			applyCommentText(critique, comments)
			return critique, nil
		}
		if attempt == maxOutputAttempts {
			return nil, fmt.Errorf("github models: invalid comments critique output: %w", err)
		}
	}
	return nil, fmt.Errorf("github models: comments critique unavailable after retries")
}

func (p *githubProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	if len(comments) == 0 {
		return &generator.CommentsCritique{
			Summary:  "No comments to analyze.",
			Comments: []generator.AnalyzedComment{},
		}, nil
	}

	if p.mode == config.ModelModeRoundRobin {
		m := p.pickModel()
		critique, err := p.tryCommentsModel(m.model, m.settings, title, articleURL, comments)
		if err != nil {
			return nil, fmt.Errorf("github models comments analysis: %w", err)
		}
		return critique, nil
	}

	// Default: fallback mode — try each model in order, moving on for HTTP 429.
	var lastRateLimitErr error
	for i, m := range p.allModels() {
		critique, err := p.tryCommentsModel(m.model, m.settings, title, articleURL, comments)
		if err == nil {
			if i > 0 && lastRateLimitErr != nil {
				log.Printf("github models comments analysis: fallback model %q succeeded after earlier rate-limit", m.model)
			}
			return critique, nil
		}
		var rateLimitErr *ErrRateLimit
		if errors.As(err, &rateLimitErr) {
			log.Printf("github models comments analysis: model %q rate-limited; trying next fallback model", m.model)
			lastRateLimitErr = err
			continue // try next fallback model
		}
		return nil, fmt.Errorf("github models comments analysis: %w", err)
	}
	return nil, fmt.Errorf("github models comments analysis: all models rate limited: %w", lastRateLimitErr)
}
