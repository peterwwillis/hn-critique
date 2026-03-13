// Package ai provides AI-backed analyzers for HN articles and comments.
// Multiple backend providers are supported: OpenAI (and any OpenAI-compatible
// server such as Ollama or llama-server), and GitHub Models.
package ai

import (
	"fmt"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

// Provider analyses HN content using an AI backend.
type Provider interface {
	// AnalyzeArticle generates a factual critique of an article.
	AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error)

	// AnalyzeComments generates a ranked critique of a story's comment section.
	AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error)

	// Name returns a human-readable identifier for the provider, used in logs.
	Name() string
}

// NewProvider constructs the Provider configured by cfg.
// It returns an error when cfg.Validate() fails or the provider is unknown.
func NewProvider(cfg *config.Config) (Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.Provider {
	case config.ProviderOpenAI:
		return newOpenAIProvider(cfg.OpenAI), nil
	case config.ProviderOllama:
		// Ollama exposes an OpenAI-compatible /v1/chat/completions endpoint.
		// Route through the unified openai provider using the Ollama base URL
		// and model, without an API key (Ollama does not require authentication).
		return newOpenAIProvider(config.OpenAIConfig{
			BaseURL:         cfg.Ollama.BaseURL,
			ChatModel:       cfg.Ollama.Model,
			UseResponsesAPI: false,
		}), nil
	case config.ProviderGitHub:
		return newGitHubProvider(cfg.GitHub), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}
