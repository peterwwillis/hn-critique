package ai

import (
	"testing"

	"github.com/peterwwillis/hn-critique/internal/config"
)

// TestGitHubRoundRobinPickModel verifies that pickModel cycles through all
// configured models in sequence using atomic increments.
func TestGitHubRoundRobinPickModel(t *testing.T) {
	p := &githubProvider{
		model:    "model-a",
		settings: config.DefaultModelConfig(),
		fallbacks: []githubFallback{
			{model: "model-b", settings: config.DefaultModelConfig()},
			{model: "model-c", settings: config.DefaultModelConfig()},
		},
		mode: config.ModelModeRoundRobin,
	}

	all := p.allModels()
	if len(all) != 3 {
		t.Fatalf("allModels length = %d, want 3", len(all))
	}

	// Verify round-robin cycles through all three models and wraps around.
	expected := []string{"model-a", "model-b", "model-c", "model-a", "model-b"}
	for i, want := range expected {
		got := p.pickModel()
		if got.model != want {
			t.Errorf("pick %d: got %q, want %q", i, got.model, want)
		}
	}
}

// TestGitHubRoundRobinSingleModel verifies that pickModel returns the only
// configured model when no fallbacks are set.
func TestGitHubRoundRobinSingleModel(t *testing.T) {
	p := &githubProvider{
		model:    "solo-model",
		settings: config.DefaultModelConfig(),
		mode:     config.ModelModeRoundRobin,
	}

	for i := range 5 {
		got := p.pickModel()
		if got.model != "solo-model" {
			t.Errorf("pick %d: got %q, want %q", i, got.model, "solo-model")
		}
	}
}

// TestOpenAIRoundRobinPickChatModel verifies that pickChatModel cycles through
// all configured chat models in sequence.
func TestOpenAIRoundRobinPickChatModel(t *testing.T) {
	settings := config.DefaultModelConfig()
	p := &openAIProvider{
		chatModel:  "model-a",
		chatModels: []string{"model-a", "model-b", "model-c"},
		allChatSettings: []config.ModelConfig{
			settings,
			settings,
			settings,
		},
		mode: config.ModelModeRoundRobin,
	}

	expected := []string{"model-a", "model-b", "model-c", "model-a", "model-b"}
	for i, want := range expected {
		model, _ := p.pickChatModel()
		if model != want {
			t.Errorf("pick %d: got %q, want %q", i, model, want)
		}
	}
}

// TestOpenAIRoundRobinSettingsMatch verifies that pickChatModel returns the
// model settings that correspond to the chosen model index.
func TestOpenAIRoundRobinSettingsMatch(t *testing.T) {
	maxA := 1000
	maxB := 2000
	settingsA := config.DefaultModelConfig()
	settingsA.Inference.MaxOutputTokens = &maxA
	settingsB := config.DefaultModelConfig()
	settingsB.Inference.MaxOutputTokens = &maxB

	p := &openAIProvider{
		chatModel:       "model-a",
		chatModels:      []string{"model-a", "model-b"},
		allChatSettings: []config.ModelConfig{settingsA, settingsB},
		mode:            config.ModelModeRoundRobin,
	}

	// First pick → model-a with settingsA.
	model, settings := p.pickChatModel()
	if model != "model-a" {
		t.Errorf("first pick model = %q, want %q", model, "model-a")
	}
	if settings.Inference.MaxOutputTokens == nil || *settings.Inference.MaxOutputTokens != maxA {
		t.Errorf("first pick settings MaxOutputTokens = %v, want %d", settings.Inference.MaxOutputTokens, maxA)
	}

	// Second pick → model-b with settingsB.
	model, settings = p.pickChatModel()
	if model != "model-b" {
		t.Errorf("second pick model = %q, want %q", model, "model-b")
	}
	if settings.Inference.MaxOutputTokens == nil || *settings.Inference.MaxOutputTokens != maxB {
		t.Errorf("second pick settings MaxOutputTokens = %v, want %d", settings.Inference.MaxOutputTokens, maxB)
	}
}
