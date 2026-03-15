package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

const defaultOpenAIBaseURL = "https://api.openai.com"

type openAIProvider struct {
	chatEndpoint      string
	responsesEndpoint string
	apiKey            string
	chatModel         string
	searchModel       string
	useResponsesAPI   bool
	chatSettings      config.ModelConfig
	searchSettings    config.ModelConfig
	http              *http.Client
}

// openAIConfig builds an OpenAIConfig from just an API key, using defaults for
// other fields. Used by the backward-compat NewAnalyzer function.
func openAIConfig(apiKey string) config.OpenAIConfig {
	return config.OpenAIConfig{
		APIKey:          apiKey,
		ChatModel:       "gpt-4o-mini",
		SearchModel:     "gpt-4o-mini",
		UseResponsesAPI: true,
	}
}

func newOpenAIProvider(cfg config.OpenAIConfig, chatSettings, searchSettings config.ModelConfig) *openAIProvider {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	return &openAIProvider{
		chatEndpoint:      base + "/v1/chat/completions",
		responsesEndpoint: base + "/v1/responses",
		apiKey:            cfg.APIKey,
		chatModel:         cfg.ChatModel,
		searchModel:       cfg.SearchModel,
		useResponsesAPI:   cfg.UseResponsesAPI,
		chatSettings:      chatSettings,
		searchSettings:    searchSettings,
		http:              &http.Client{Timeout: httpTimeout},
	}
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	var text string
	var err error

	if p.useResponsesAPI {
		prompt := articlePrompt(title, articleURL, content, p.searchSettings.Limits.ArticlePromptBytes)
		text, err = p.callResponsesAPI(prompt, p.searchSettings.Inference)
		if err != nil {
			// Fall back to Chat Completions when the Responses API is unavailable.
			prompt = articlePrompt(title, articleURL, content, p.chatSettings.Limits.ArticlePromptBytes)
			text, err = callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), p.chatModel, prompt, true, p.chatSettings.Inference)
		}
	} else {
		prompt := articlePrompt(title, articleURL, content, p.chatSettings.Limits.ArticlePromptBytes)
		text, err = callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), p.chatModel, prompt, true, p.chatSettings.Inference)
	}
	if err != nil {
		return nil, fmt.Errorf("openai article analysis: %w", err)
	}

	var critique generator.ArticleCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("openai: parsing article critique: %w", err)
	}
	critique.Rating = sanitizeRating(critique.Rating)
	return &critique, nil
}

func (p *openAIProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	prompt := commentsPrompt(title, articleURL, buildCommentText(comments, p.chatSettings.Limits.CommentPromptBytes))

	text, err := callChatCompletions(p.http, p.chatEndpoint, bearerHeader(p.apiKey), p.chatModel, prompt, true, p.chatSettings.Inference)
	if err != nil {
		return nil, fmt.Errorf("openai comments analysis: %w", err)
	}

	var critique generator.CommentsCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("openai: parsing comments critique: %w", err)
	}
	applyCommentText(&critique, comments)
	return &critique, nil
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

// callResponsesAPI calls the OpenAI Responses API with the web_search_preview tool.
func (p *openAIProvider) callResponsesAPI(input string, inference config.InferenceConfig) (string, error) {
	payload := map[string]any{
		"model": p.searchModel,
		"tools": []map[string]string{{"type": "web_search_preview"}},
		"input": input,
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
