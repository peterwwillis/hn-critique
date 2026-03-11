package ai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

// ollamaProvider calls a local Ollama server using the OpenAI-compatible
// /v1/chat/completions endpoint exposed by Ollama's OpenAI compatibility layer.
// See https://ollama.com/blog/openai-compatibility for details.
type ollamaProvider struct {
	endpoint string // full URL to /v1/chat/completions
	model    string
	http     *http.Client
}

func newOllamaProvider(cfg config.OllamaConfig) *ollamaProvider {
	base := strings.TrimRight(cfg.BaseURL, "/")
	return &ollamaProvider{
		endpoint: base + "/v1/chat/completions",
		model:    cfg.Model,
		http:     &http.Client{Timeout: httpTimeout},
	}
}

func (p *ollamaProvider) Name() string { return "ollama" }

func (p *ollamaProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	prompt := articlePrompt(title, articleURL, content)

	// Ollama's OpenAI-compatible layer does not require an Authorization header,
	// but we pass an empty bearer so callChatCompletions stays generic.
	text, err := callChatCompletions(p.http, p.endpoint, "Bearer ollama", p.model, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("ollama article analysis: %w", err)
	}

	var critique generator.ArticleCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("ollama: parsing article critique: %w", err)
	}
	critique.Rating = sanitizeRating(critique.Rating)
	return &critique, nil
}

func (p *ollamaProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments))

	text, err := callChatCompletions(p.http, p.endpoint, "Bearer ollama", p.model, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("ollama comments analysis: %w", err)
	}

	var critique generator.CommentsCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("ollama: parsing comments critique: %w", err)
	}
	return &critique, nil
}
