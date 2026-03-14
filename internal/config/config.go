// Package config loads and validates the hn-critique TOML configuration file.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ProviderName is the identifier for an AI provider.
type ProviderName string

const (
	ProviderOpenAI ProviderName = "openai"
	ProviderOllama ProviderName = "ollama"
	ProviderGitHub ProviderName = "github"
)

// Config is the top-level configuration loaded from a TOML file.
type Config struct {
	// Provider selects which AI backend to use (openai, ollama, github).
	// Can be overridden by the -provider flag on the command line.
	Provider ProviderName `toml:"provider"`

	OpenAI OpenAIConfig `toml:"openai"`
	Ollama OllamaConfig `toml:"ollama"`
	GitHub GitHubConfig `toml:"github"`
}

// OpenAIConfig holds settings for the OpenAI provider.
// The same provider can target any OpenAI-compatible backend (Ollama,
// llama-server, LM Studio, vLLM, …) by setting BaseURL.
type OpenAIConfig struct {
	// APIKey falls back to the OPENAI_API_KEY environment variable when empty.
	// For local/private backends that do not require authentication, leave
	// both APIKey and the environment variable unset; validation is relaxed
	// when BaseURL points to a non-default host.
	APIKey string `toml:"api_key"`
	// BaseURL is the root URL of an OpenAI-compatible inference server.
	// Defaults to https://api.openai.com when empty.
	// Falls back to the OPENAI_BASE_URL environment variable when empty.
	// Examples:
	//   http://localhost:11434   (Ollama)
	//   http://localhost:8080    (llama-server / llama.cpp)
	//   http://192.168.1.50:1234 (LM Studio on another machine)
	BaseURL string `toml:"base_url"`
	// ChatModel is the model used for chat completions.
	// Falls back to the OPENAI_CHAT_MODEL environment variable when empty.
	ChatModel string `toml:"chat_model"`
	// SearchModel is the model used when web search is requested via the Responses API.
	SearchModel string `toml:"search_model"`
	// UseResponsesAPI enables the Responses API (with web_search_preview) for
	// article analysis. Falls back to Chat Completions when false or unavailable.
	// This feature is specific to api.openai.com and should be false for other backends.
	UseResponsesAPI bool `toml:"use_responses_api"`
}

// OllamaConfig holds settings for a local Ollama instance.
//
// Deprecated: configure the openai provider with base_url pointing to your
// Ollama server instead.  The ollama provider now routes through the same
// OpenAI-compatible HTTP client used by the openai provider, so there is no
// behavioural difference.  This section is kept for backward compatibility and
// will be removed in a future version.
type OllamaConfig struct {
	// BaseURL is the Ollama server root URL. Defaults to http://localhost:11434.
	BaseURL string `toml:"base_url"`
	// Model is the Ollama model name to use (e.g. "llama3.2", "mistral").
	Model string `toml:"model"`
}

// GitHubConfig holds settings for the GitHub Models provider.
type GitHubConfig struct {
	// Token is the GitHub token used to authenticate requests.
	// Falls back to the GITHUB_TOKEN environment variable when empty.
	// In GitHub Actions add `permissions: models: read` to the workflow job.
	Token string `toml:"token"`
	// Endpoint is the base URL for the GitHub Models inference API.
	// Defaults to https://models.github.ai/inference.
	Endpoint string `toml:"endpoint"`
	// Model is the model identifier in the format "provider/model-name"
	// (e.g. "openai/gpt-4o-mini", "openai/gpt-4.1").
	Model string `toml:"model"`
}

// Defaults returns a Config pre-filled with sensible default values.
func Defaults() *Config {
	return &Config{
		Provider: ProviderGitHub,
		OpenAI: OpenAIConfig{
			ChatModel:       "gpt-4o-mini",
			SearchModel:     "gpt-4o-mini",
			UseResponsesAPI: true,
		},
		Ollama: OllamaConfig{
			BaseURL: "http://localhost:11434",
			Model:   "llama3.2",
		},
		GitHub: GitHubConfig{
			Endpoint: "https://models.github.ai/inference",
			Model:    "openai/gpt-4o-mini",
		},
	}
}

// Load reads the TOML file at path, merges it over the defaults, and resolves
// credential fields from environment variables.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		if _, err := toml.DecodeFile(path, cfg); err != nil {
			return nil, fmt.Errorf("loading config %q: %w", path, err)
		}
	}

	// Resolve credentials and settings from environment variables if not set in file.
	if cfg.OpenAI.APIKey == "" {
		cfg.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.OpenAI.BaseURL == "" {
		cfg.OpenAI.BaseURL = os.Getenv("OPENAI_BASE_URL")
	}
	// OPENAI_CHAT_MODEL overrides the default and config-file value so that
	// the model can be configured purely via an environment variable (e.g. in
	// GitHub Actions without committing a config file).
	if v := os.Getenv("OPENAI_CHAT_MODEL"); v != "" {
		cfg.OpenAI.ChatModel = v
	}
	if cfg.GitHub.Token == "" {
		cfg.GitHub.Token = os.Getenv("GITHUB_TOKEN")
	}

	return cfg, nil
}

// Validate checks that the selected provider has enough configuration to
// operate. It returns a descriptive error when a required credential is missing.
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderOpenAI:
		// Require an API key only when targeting the default OpenAI endpoint.
		// Local / private backends (Ollama, llama-server, …) typically do not
		// need authentication.
		if c.OpenAI.APIKey == "" && c.OpenAI.BaseURL == "" {
			return errors.New("provider \"openai\" requires api_key (or OPENAI_API_KEY env var); " +
				"set base_url (or OPENAI_BASE_URL) if you are targeting a local backend that does not need a key")
		}
	case ProviderOllama:
		if c.Ollama.BaseURL == "" {
			return errors.New("provider \"ollama\" requires base_url")
		}
		if c.Ollama.Model == "" {
			return errors.New("provider \"ollama\" requires model")
		}
	case ProviderGitHub:
		if c.GitHub.Token == "" {
			return errors.New("provider \"github\" requires token (or GITHUB_TOKEN env var)")
		}
		if c.GitHub.Endpoint == "" {
			return errors.New("provider \"github\" requires endpoint")
		}
		if c.GitHub.Model == "" {
			return errors.New("provider \"github\" requires model")
		}
	default:
		return fmt.Errorf("unknown provider %q (must be one of: openai, ollama, github)", c.Provider)
	}
	return nil
}
