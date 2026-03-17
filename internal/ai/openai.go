package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

const defaultOpenAIBaseURL = "https://api.openai.com"

type openAIProvider struct {
	chatEndpoint      string
	responsesEndpoint string
	apiKey            string
	// chatModel is the primary (or only) model for chat completions.
	chatModel string
	// chatModels is the ordered list of models to use for chat completions.
	// The first entry is the primary; subsequent entries are used for
	// fallback or round-robin depending on mode.
	chatModels     []string
	searchModel    string
	useResponsesAPI bool
	mode            config.ModelMode
	counter         atomic.Uint64
	chatSettings    config.ModelConfig
	searchSettings  config.ModelConfig
	allChatSettings []config.ModelConfig // parallel to chatModels
	http            *http.Client
}

// openAIConfig builds an OpenAIConfig from just an API key, using defaults for
// other fields. Used by the backward-compat NewAnalyzer function.
func openAIConfig(apiKey string) config.OpenAIConfig {
	d := config.Defaults()
	return config.OpenAIConfig{
		APIKey:          apiKey,
		ChatModel:       d.OpenAI.ChatModel,
		SearchModel:     d.OpenAI.SearchModel,
		UseResponsesAPI: d.OpenAI.UseResponsesAPI,
	}
}

func newOpenAIProvider(cfg config.OpenAIConfig, chatSettings, searchSettings config.ModelConfig, extraChatModels []string, extraChatSettings []config.ModelConfig) *openAIProvider {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}

	// Build the complete ordered list of chat models and their settings.
	// The primary model always comes first.
	chatModels := make([]string, 0, 1+len(extraChatModels))
	chatModels = append(chatModels, cfg.ChatModel)
	chatModels = append(chatModels, extraChatModels...)

	allChatSettings := make([]config.ModelConfig, 0, 1+len(extraChatSettings))
	allChatSettings = append(allChatSettings, chatSettings)
	allChatSettings = append(allChatSettings, extraChatSettings...)

	return &openAIProvider{
		chatEndpoint:      base + "/v1/chat/completions",
		responsesEndpoint: base + "/v1/responses",
		apiKey:            cfg.APIKey,
		chatModel:         cfg.ChatModel,
		chatModels:        chatModels,
		searchModel:       cfg.SearchModel,
		useResponsesAPI:   cfg.UseResponsesAPI,
		mode:              cfg.ModelMode,
		chatSettings:      chatSettings,
		searchSettings:    searchSettings,
		allChatSettings:   allChatSettings,
		http:              newHTTPClient(),
	}
}

func (p *openAIProvider) Name() string { return "openai" }

// pickChatModel returns the model and settings to use in round-robin mode.
func (p *openAIProvider) pickChatModel() (string, config.ModelConfig) {
	idx := p.counter.Add(1) - 1
	i := int(idx % uint64(len(p.chatModels)))
	return p.chatModels[i], p.allChatSettings[i]
}

func (p *openAIProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	if p.mode == config.ModelModeRoundRobin {
		model, settings := p.pickChatModel()
		return p.analyzeArticleWithModel(model, settings, title, articleURL, content)
	}

	// Default: fallback mode — try each model in order, moving on for HTTP 429.
	var lastRateLimitErr error
	for i, model := range p.chatModels {
		settings := p.allChatSettings[i]
		critique, err := p.analyzeArticleWithModel(model, settings, title, articleURL, content)
		if err == nil {
			if i > 0 && lastRateLimitErr != nil {
				log.Printf("openai article analysis: fallback model %q succeeded after earlier rate-limit", model)
			}
			return critique, nil
		}
		var rateLimitErr *ErrRateLimit
		if errors.As(err, &rateLimitErr) {
			log.Printf("openai article analysis: model %q rate-limited; trying next fallback model", model)
			lastRateLimitErr = err
			continue
		}
		return nil, err
	}
	if lastRateLimitErr != nil {
		return nil, fmt.Errorf("openai article analysis: all models rate limited: %w", lastRateLimitErr)
	}
	return nil, fmt.Errorf("openai: article critique unavailable after retries")
}

func (p *openAIProvider) analyzeArticleWithModel(model string, settings config.ModelConfig, title, articleURL, content string) (*generator.ArticleCritique, error) {
	apiModel := openAIModelID(model)
	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		var text string
		var err error

		if p.useResponsesAPI {
			prompt := articlePrompt(title, articleURL, content, p.searchSettings.Limits.ArticlePromptBytes)
			text, err = p.callResponsesAPI(prompt, p.searchSettings.Inference)
			if err != nil {
				// Fall back to Chat Completions when the Responses API is unavailable.
				prompt = articlePrompt(title, articleURL, content, settings.Limits.ArticlePromptBytes)
				text, err = callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), apiModel, prompt, true, settings.Inference)
			}
		} else {
			prompt := articlePrompt(title, articleURL, content, settings.Limits.ArticlePromptBytes)
			text, err = callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), apiModel, prompt, true, settings.Inference)
		}
		if err != nil {
			return nil, fmt.Errorf("openai article analysis: %w", err)
		}

		critique, err := parseArticleCritique(text)
		if err == nil {
			return critique, nil
		}
		if attempt == maxOutputAttempts {
			return nil, fmt.Errorf("openai: invalid article critique output: %w", err)
		}
	}
	return nil, fmt.Errorf("openai: article critique unavailable after retries")
}

func (p *openAIProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	if len(comments) == 0 {
		return &generator.CommentsCritique{
			Summary:  "No comments to analyze.",
			Comments: []generator.AnalyzedComment{},
		}, nil
	}

	if p.mode == config.ModelModeRoundRobin {
		model, settings := p.pickChatModel()
		return p.analyzeCommentsWithModel(model, settings, title, articleURL, comments)
	}

	// Default: fallback mode — try each model in order, moving on for HTTP 429.
	var lastRateLimitErr error
	for i, model := range p.chatModels {
		settings := p.allChatSettings[i]
		critique, err := p.analyzeCommentsWithModel(model, settings, title, articleURL, comments)
		if err == nil {
			if i > 0 && lastRateLimitErr != nil {
				log.Printf("openai comments analysis: fallback model %q succeeded after earlier rate-limit", model)
			}
			return critique, nil
		}
		var rateLimitErr *ErrRateLimit
		if errors.As(err, &rateLimitErr) {
			log.Printf("openai comments analysis: model %q rate-limited; trying next fallback model", model)
			lastRateLimitErr = err
			continue
		}
		return nil, err
	}
	if lastRateLimitErr != nil {
		return nil, fmt.Errorf("openai comments analysis: all models rate limited: %w", lastRateLimitErr)
	}
	return nil, fmt.Errorf("openai: comments critique unavailable after retries")
}

func (p *openAIProvider) analyzeCommentsWithModel(model string, settings config.ModelConfig, title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments, settings.Limits.CommentPromptBytes))
	apiModel := openAIModelID(model)

	for attempt := 1; attempt <= maxOutputAttempts; attempt++ {
		text, err := callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), apiModel, prompt, true, settings.Inference)
		if err != nil {
			return nil, fmt.Errorf("openai comments analysis: %w", err)
		}

		critique, err := parseCommentsCritique(text, comments)
		if err == nil {
			applyCommentText(critique, comments)
			return critique, nil
		}
		if attempt == maxOutputAttempts {
			return nil, fmt.Errorf("openai: invalid comments critique output: %w", err)
		}
	}
	return nil, fmt.Errorf("openai: comments critique unavailable after retries")
}

// bearerHeader returns the value to use for the Authorization header.
// When apiKey is empty (local backends that do not require auth) a placeholder
// value is used so that the header field is always present.
func bearerHeader(apiKey string) string {
	if apiKey == "" {
		return "Bearer none"
	}
	return "Bearer " + apiKey
}

// openAIModelID strips any leading "provider/" prefix from a model name for
// use with the OpenAI API, which expects bare model IDs (e.g. "gpt-4.1-mini")
// rather than the GitHub Models-style qualified form ("openai/gpt-4.1-mini").
func openAIModelID(model string) string {
	if idx := strings.Index(model, "/"); idx >= 0 {
		return model[idx+1:]
	}
	return model
}

// callResponsesAPI calls the OpenAI Responses API with the web_search_preview tool.
func (p *openAIProvider) callResponsesAPI(input string, inference config.InferenceConfig) (string, error) {
	payload := map[string]any{
		"model": openAIModelID(p.searchModel),
		"tools": []map[string]string{{"type": "web_search_preview"}},
		"input": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": input},
		},
	}
	if inference.Temperature != nil {
		payload["temperature"] = *inference.Temperature
	}
	if inference.MaxOutputTokens != nil {
		payload["max_output_tokens"] = *inference.MaxOutputTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", p.responsesEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", bearerHeader(p.apiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("responses API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decoding responses API: %w", err)
	}
	for _, out := range result.Output {
		if out.Type == "message" {
			for _, c := range out.Content {
				if c.Type == "output_text" && c.Text != "" {
					return c.Text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no text output from responses API")
}
