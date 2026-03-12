package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/config"
)

func TestDefaults(t *testing.T) {
	cfg := config.Defaults()

	if cfg.Provider != config.ProviderOpenAI {
		t.Errorf("default provider = %q, want %q", cfg.Provider, config.ProviderOpenAI)
	}
	if cfg.OpenAI.ChatModel == "" {
		t.Error("default OpenAI.ChatModel is empty")
	}
	if cfg.Ollama.BaseURL == "" {
		t.Error("default Ollama.BaseURL is empty")
	}
	if cfg.GitHub.Endpoint == "" {
		t.Error("default GitHub.Endpoint is empty")
	}
}

func TestLoadEmpty(t *testing.T) {
	// Load with empty path — should return defaults.
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}
	if cfg.Provider != config.ProviderOpenAI {
		t.Errorf("provider = %q, want %q", cfg.Provider, config.ProviderOpenAI)
	}
}

func TestLoadTOML(t *testing.T) {
	toml := `
provider = "ollama"

[ollama]
base_url = "http://192.168.1.100:11434"
model = "mistral"

[openai]
api_key = "sk-test"
chat_model = "gpt-4o"
use_responses_api = false
`
	path := filepath.Join(t.TempDir(), "test.toml")
	if err := os.WriteFile(path, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Provider != config.ProviderOllama {
		t.Errorf("provider = %q, want %q", cfg.Provider, config.ProviderOllama)
	}
	if cfg.Ollama.BaseURL != "http://192.168.1.100:11434" {
		t.Errorf("Ollama.BaseURL = %q", cfg.Ollama.BaseURL)
	}
	if cfg.Ollama.Model != "mistral" {
		t.Errorf("Ollama.Model = %q", cfg.Ollama.Model)
	}
	if cfg.OpenAI.APIKey != "sk-test" {
		t.Errorf("OpenAI.APIKey = %q", cfg.OpenAI.APIKey)
	}
	if cfg.OpenAI.ChatModel != "gpt-4o" {
		t.Errorf("OpenAI.ChatModel = %q", cfg.OpenAI.ChatModel)
	}
	if cfg.OpenAI.UseResponsesAPI {
		t.Error("OpenAI.UseResponsesAPI should be false")
	}
}

func TestLoadEnvFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434")
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.OpenAI.APIKey != "env-key" {
		t.Errorf("OpenAI.APIKey = %q, want %q from env", cfg.OpenAI.APIKey, "env-key")
	}
	if cfg.OpenAI.BaseURL != "http://localhost:11434" {
		t.Errorf("OpenAI.BaseURL = %q, want %q from OPENAI_BASE_URL", cfg.OpenAI.BaseURL, "http://localhost:11434")
	}
	if cfg.GitHub.Token != "ghp_test" {
		t.Errorf("GitHub.Token = %q, want %q from env", cfg.GitHub.Token, "ghp_test")
	}
}

func TestValidateOpenAI(t *testing.T) {
	cfg := config.Defaults()
	cfg.Provider = config.ProviderOpenAI

	// Without key and without custom base URL — should fail.
	cfg.OpenAI.APIKey = ""
	cfg.OpenAI.BaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing OpenAI API key")
	}

	// With key — should pass.
	cfg.OpenAI.APIKey = "sk-test"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// With a custom base URL but no key — should pass (local backend).
	cfg.OpenAI.APIKey = ""
	cfg.OpenAI.BaseURL = "http://localhost:11434"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error with custom base_url and no key: %v", err)
	}
}

func TestValidateOllama(t *testing.T) {
	cfg := config.Defaults()
	cfg.Provider = config.ProviderOllama

	// Defaults should already have valid Ollama settings.
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error with defaults: %v", err)
	}

	cfg.Ollama.BaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing Ollama base_url")
	}
}

func TestValidateGitHub(t *testing.T) {
	cfg := config.Defaults()
	cfg.Provider = config.ProviderGitHub

	// Without token — should fail.
	cfg.GitHub.Token = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing GitHub token")
	}

	// With token — should pass.
	cfg.GitHub.Token = "ghp_test"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateUnknownProvider(t *testing.T) {
	cfg := config.Defaults()
	cfg.Provider = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestLoadBadTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("this is not valid toml }{}{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error loading invalid TOML file")
	}
}
