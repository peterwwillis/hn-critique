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
		// When ChatModels is set, use its first entry as the primary model.
		// This lets users specify the full ordered list in one place.
		if len(cfg.OpenAI.ChatModels) > 0 {
			cfg.OpenAI.ChatModel = cfg.OpenAI.ChatModels[0]
		}
		// Build the ordered list of extra chat models (beyond the primary ChatModel).
		extraModels, extraSettings := openAIExtraModels(cfg)
		return newOpenAIProvider(
			cfg.OpenAI,
			cfg.ModelConfigFor(cfg.OpenAI.ChatModel),
			cfg.ModelConfigFor(cfg.OpenAI.SearchModel),
			extraModels,
			extraSettings,
		), nil
	case config.ProviderOllama:
		// Ollama exposes an OpenAI-compatible /v1/chat/completions endpoint.
		// Route through the unified openai provider using the Ollama base URL
		// and model, without an API key (Ollama does not require authentication).
		modelSettings := cfg.ModelConfigFor(cfg.Ollama.Model)
		return newOpenAIProvider(config.OpenAIConfig{
			BaseURL:         cfg.Ollama.BaseURL,
			ChatModel:       cfg.Ollama.Model,
			SearchModel:     cfg.Ollama.Model,
			UseResponsesAPI: false,
		}, modelSettings, modelSettings, nil, nil), nil
	case config.ProviderGitHub:
		settings := cfg.ModelConfigFor(cfg.GitHub.Model)
		fallbacks := make([]githubFallback, 0, len(cfg.GitHub.FallbackModels))
		for _, name := range cfg.GitHub.FallbackModels {
			fallbacks = append(fallbacks, githubFallback{
				model:    name,
				settings: cfg.ModelConfigFor(name),
			})
		}
		return newGitHubProvider(cfg.GitHub, settings, fallbacks), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

// openAIExtraModels builds the extra (non-primary) model list for the OpenAI
// provider from cfg.OpenAI.ChatModels. When ChatModels is set, it is treated
// as the complete ordered list of models: the first entry overrides ChatModel
// as the primary, and subsequent entries are the additional models used for
// fallback or round-robin selection.
func openAIExtraModels(cfg *config.Config) ([]string, []config.ModelConfig) {
	if len(cfg.OpenAI.ChatModels) == 0 {
		return nil, nil
	}
	// When ChatModels is provided, it is the complete list. The first entry is
	// the primary (already set via cfg.OpenAI.ChatModel or used directly), and
	// everything after it is extra.
	extra := cfg.OpenAI.ChatModels[1:]
	models := make([]string, 0, len(extra))
	settings := make([]config.ModelConfig, 0, len(extra))
	for _, name := range extra {
		models = append(models, name)
		settings = append(settings, cfg.ModelConfigFor(name))
	}
	return models, settings
}
