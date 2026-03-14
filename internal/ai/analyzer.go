// Package ai provides AI-backed analyzers for HN articles and comments.
// See provider.go for the Provider interface and NewProvider factory.
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
	maxCommentChars = 6000
	maxArticleChars = 6000
	httpTimeout     = 120 * time.Second
)

// articlePrompt builds the fact-checking prompt for an article.
func articlePrompt(title, articleURL, content string) string {
	if len(content) > maxArticleChars {
		content = content[:maxArticleChars] + "…"
	}
	return fmt.Sprintf(`You are a critical fact-checker. Analyze the following article and respond with ONLY a valid JSON object — no markdown, no code fences, just raw JSON.

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
}

// commentsPrompt builds the analysis prompt for a comment section.
func commentsPrompt(title, articleURL, commentLines string) string {
	return fmt.Sprintf(`You are a critical analyst. Analyze the following Hacker News comment section and respond with ONLY a valid JSON object — no markdown, no code fences, just raw JSON.

The JSON must have exactly this shape:
{
  "summary": "<2-3 sentence overview of the discussion>",
  "comments": [
    {
      "id": <comment id as integer>,
      "author": "<username>",
      "text": "<full comment text as provided above, plain text>",
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
}

// sanitizeRating ensures the rating field has a valid value.
func sanitizeRating(r string) string {
	switch r {
	case "reliable", "questionable", "misleading":
		return r
	default:
		return "questionable"
	}
}

// buildCommentText formats comments for the AI prompt.
func buildCommentText(comments []*generator.Comment) string {
	var sb strings.Builder
	for _, c := range comments {
		entry := fmt.Sprintf("[id:%d by:%s]\n%s\n\n", c.ID, c.Author, c.Text)
		if sb.Len() == 0 && len(entry) > maxCommentChars {
			// Always include the first comment even if it exceeds the limit so we do not truncate it.
			sb.WriteString(entry)
			break
		}
		if sb.Len()+len(entry) > maxCommentChars {
			break
		}
		sb.WriteString(entry)
	}
	return sb.String()
}

func applyCommentText(critique *generator.CommentsCritique, comments []*generator.Comment) {
	if critique == nil || len(critique.Comments) == 0 || len(comments) == 0 {
		return
	}

	commentByID := make(map[int]*generator.Comment, len(comments))
	stack := append([]*generator.Comment(nil), comments...)
	for len(stack) > 0 {
		comment := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		commentByID[comment.ID] = comment
		if len(comment.Kids) > 0 {
			stack = append(stack, comment.Kids...)
		}
	}

	for i := range critique.Comments {
		if original, ok := commentByID[critique.Comments[i].ID]; ok {
			critique.Comments[i].Text = string(original.Text)
		}
	}
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

// chatRequest is the standard OpenAI-compatible chat completions request body.
type chatRequest struct {
	Model          string              `json:"model"`
	Messages       []map[string]string `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *responseFormat     `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// callChatCompletions sends a POST to an OpenAI-compatible chat completions
// endpoint and returns the first choice's content text.
// endpoint must be the full URL including path (e.g. ".../v1/chat/completions").
func callChatCompletions(httpClient *http.Client, endpoint, authHeader, model, prompt string, jsonMode bool) (string, error) {
	req := chatRequest{
		Model: model,
		Messages: []map[string]string{
			{"role": "user", "content": prompt},
		},
		Temperature: 0.3,
	}
	if jsonMode {
		req.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", authHeader)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API HTTP %d at %s: %s", resp.StatusCode, endpoint, string(respBody))
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
		return "", fmt.Errorf("no choices in chat response from %s", endpoint)
	}
	return result.Choices[0].Message.Content, nil
}

// Analyzer is retained for backward compatibility. New code should use NewProvider.
// It wraps the OpenAI provider.
type Analyzer struct {
	p Provider
}

// NewAnalyzer creates an Analyzer backed by the OpenAI provider.
// Deprecated: use NewProvider with a config.Config instead.
func NewAnalyzer(apiKey string) *Analyzer {
	p := newOpenAIProvider(openAIConfig(apiKey))
	return &Analyzer{p: p}
}

// AnalyzeArticle delegates to the underlying Provider.
func (a *Analyzer) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	return a.p.AnalyzeArticle(title, articleURL, content)
}

// AnalyzeComments delegates to the underlying Provider.
func (a *Analyzer) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	return a.p.AnalyzeComments(title, articleURL, comments)
}
