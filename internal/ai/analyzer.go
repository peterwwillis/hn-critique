// Package ai provides an OpenAI-backed analyzer for HN articles and comments.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

const (
	responsesEndpoint   = "https://api.openai.com/v1/responses"
	chatEndpoint        = "https://api.openai.com/v1/chat/completions"
	defaultModel        = "gpt-4o-mini"
	searchModel         = "gpt-4o-mini"
	maxCommentChars     = 6000
	maxArticleChars     = 6000
)

// Analyzer uses the OpenAI API to generate critiques.
type Analyzer struct {
	apiKey string
	http   *http.Client
}

// NewAnalyzer creates a new Analyzer with the given OpenAI API key.
func NewAnalyzer(apiKey string) *Analyzer {
	return &Analyzer{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 120 * time.Second},
	}
}

// AnalyzeArticle generates an ArticleCritique for the given article using web search.
func (a *Analyzer) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	// Trim content to keep request cost reasonable.
	if len(content) > maxArticleChars {
		content = content[:maxArticleChars] + "…"
	}

	prompt := fmt.Sprintf(`You are a critical fact-checker. Analyze the following article and respond with ONLY a valid JSON object — no markdown, no code fences, just raw JSON.

The JSON must have exactly these keys:
{
  "summary": "<2-3 sentence summary of the article>",
  "mainPoints": ["<point 1>", "<point 2>", "..."],
  "truthfulness": "<paragraph assessing the accuracy and truthfulness of the claims>",
  "considerations": ["<important consideration not mentioned in the article>", "..."],
  "rating": "<one of: reliable, questionable, misleading>"
}

Use web search to verify factual claims where possible.

Article title: %s
Article URL: %s
Article content:
%s`, title, articleURL, content)

	// Try Responses API with web search first.
	text, err := a.callResponsesAPI(prompt)
	if err != nil {
		// Fall back to standard Chat Completions.
		text, err = a.callChatCompletions(prompt, true)
		if err != nil {
			return nil, fmt.Errorf("article analysis failed: %w", err)
		}
	}

	var critique generator.ArticleCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("parsing article critique: %w", err)
	}
	// Sanitize rating value.
	switch critique.Rating {
	case "reliable", "questionable", "misleading":
	default:
		critique.Rating = "questionable"
	}
	return &critique, nil
}

// AnalyzeComments generates a CommentsCritique for the story's comment section.
func (a *Analyzer) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	commentLines := buildCommentText(comments)
	if len(commentLines) > maxCommentChars {
		commentLines = commentLines[:maxCommentChars] + "…"
	}

	prompt := fmt.Sprintf(`You are a critical analyst. Analyze the following Hacker News comment section and respond with ONLY a valid JSON object — no markdown, no code fences, just raw JSON.

The JSON must have exactly this shape:
{
  "summary": "<2-3 sentence overview of the discussion>",
  "comments": [
    {
      "id": <comment id as integer>,
      "author": "<username>",
      "text": "<first 120 chars of the comment, plain text>",
      "indicators": ["<one or more of: emotional, intelligent, thoughtful, trolling, likely-true, likely-untrue, belligerent, constructive, useless>"],
      "accuracyRank": <integer starting at 1 for most accurate>,
      "analysis": "<1-2 sentence critique>"
    }
  ]
}

Include ALL top-level comments provided. Rank them from most accurate (1) to least accurate.

Article: %s (%s)
Comments:
%s`, title, articleURL, commentLines)

	text, err := a.callChatCompletions(prompt, true)
	if err != nil {
		return nil, fmt.Errorf("comments analysis failed: %w", err)
	}

	var critique generator.CommentsCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, fmt.Errorf("parsing comments critique: %w", err)
	}
	return &critique, nil
}

// callResponsesAPI calls the OpenAI Responses API with the web_search_preview tool.
func (a *Analyzer) callResponsesAPI(input string) (string, error) {
	payload := map[string]any{
		"model": searchModel,
		"tools": []map[string]string{{"type": "web_search_preview"}},
		"input": input,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", responsesEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
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

	// Parse the Responses API output format.
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

// callChatCompletions calls the OpenAI Chat Completions endpoint.
// If jsonMode is true, the response_format is set to json_object.
func (a *Analyzer) callChatCompletions(prompt string, jsonMode bool) (string, error) {
	reqBody := map[string]any{
		"model": defaultModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}
	if jsonMode {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", chatEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decoding chat response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in chat response")
	}
	return result.Choices[0].Message.Content, nil
}

// buildCommentText formats comments for the AI prompt.
func buildCommentText(comments []*generator.Comment) string {
	var sb strings.Builder
	for _, c := range comments {
		sb.WriteString(fmt.Sprintf("[id:%d by:%s]\n%s\n\n", c.ID, c.Author, c.Text))
	}
	return sb.String()
}

// parseJSON extracts JSON from the model response and decodes it into v.
// It handles responses that wrap JSON in markdown code fences.
func parseJSON(text string, v any) error {
	text = strings.TrimSpace(text)

	// Strip markdown code fences if present.
	if idx := strings.Index(text, "```json"); idx != -1 {
		text = text[idx+7:]
		if end := strings.Index(text, "```"); end != -1 {
			text = text[:end]
		}
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if end := strings.Index(text, "```"); end != -1 {
			text = text[:end]
		}
	}
	text = strings.TrimSpace(text)

	// Find the first '{' to skip any preamble text.
	if start := strings.IndexByte(text, '{'); start > 0 {
		text = text[start:]
	}

	return json.Unmarshal([]byte(text), v)
}

// ParseJSON is the exported variant of parseJSON for use in tests.
var ParseJSON = parseJSON
