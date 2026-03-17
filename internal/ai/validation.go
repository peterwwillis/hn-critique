package ai

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

const maxCommentSnippetChars = 200

var allowedIndicators = map[string]struct{}{
	"emotional":     {},
	"intelligent":   {},
	"thoughtful":    {},
	"trolling":      {},
	"likely-true":   {},
	"likely-untrue": {},
	"belligerent":   {},
	"constructive":  {},
	"useless":       {},
}

var allowedAIRatings = map[string]struct{}{
	"reliable":       {},
	"needs citation": {},
	"questionable":   {},
	"misleading":     {},
	"opinion":        {},
}

func parseArticleCritique(text string) (*generator.ArticleCritique, error) {
	var critique generator.ArticleCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, err
	}
	if err := validateArticleCritique(&critique); err != nil {
		return nil, err
	}
	return &critique, nil
}

func parseCommentsCritique(text string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	var critique generator.CommentsCritique
	if err := parseJSON(text, &critique); err != nil {
		return nil, err
	}
	if err := validateCommentsCritique(&critique, comments); err != nil {
		return nil, err
	}
	return &critique, nil
}

func validateArticleCritique(c *generator.ArticleCritique) error {
	if c == nil {
		return fmt.Errorf("article critique is nil")
	}
	if strings.TrimSpace(c.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if strings.TrimSpace(c.Truthfulness) == "" {
		return fmt.Errorf("truthfulness is required")
	}
	if len(c.MainPoints) == 0 {
		return fmt.Errorf("mainPoints is required")
	}
	for i, point := range c.MainPoints {
		if strings.TrimSpace(point) == "" {
			return fmt.Errorf("mainPoints[%d] is empty", i)
		}
	}
	if len(c.Considerations) == 0 {
		return fmt.Errorf("considerations is required")
	}
	for i, consideration := range c.Considerations {
		if strings.TrimSpace(consideration) == "" {
			return fmt.Errorf("considerations[%d] is empty", i)
		}
	}
	rating := strings.ToLower(strings.TrimSpace(c.Rating))
	if _, ok := allowedAIRatings[rating]; !ok {
		return fmt.Errorf("rating %q is invalid", c.Rating)
	}
	c.Rating = rating
	return nil
}

func validateCommentsCritique(c *generator.CommentsCritique, expected []*generator.Comment) error {
	if c == nil {
		return fmt.Errorf("comments critique is nil")
	}
	if strings.TrimSpace(c.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	expectedIDs := make(map[int]struct{}, len(expected))
	for _, comment := range expected {
		expectedIDs[comment.ID] = struct{}{}
	}
	if len(c.Comments) != len(expectedIDs) {
		return fmt.Errorf("expected %d comments, got %d", len(expectedIDs), len(c.Comments))
	}

	ranks := make(map[int]struct{}, len(expectedIDs))
	seenIDs := make(map[int]struct{}, len(expectedIDs))
	for i := range c.Comments {
		comment := &c.Comments[i]
		if comment.ID <= 0 {
			return fmt.Errorf("comment id %d is invalid", comment.ID)
		}
		if _, ok := expectedIDs[comment.ID]; !ok {
			return fmt.Errorf("comment id %d is not expected", comment.ID)
		}
		if _, dup := seenIDs[comment.ID]; dup {
			return fmt.Errorf("comment id %d is duplicated", comment.ID)
		}
		seenIDs[comment.ID] = struct{}{}
		if strings.TrimSpace(comment.Author) == "" {
			return fmt.Errorf("comment id %d has empty author", comment.ID)
		}
		text := strings.TrimSpace(comment.Text)
		if text == "" {
			return fmt.Errorf("comment id %d has empty text", comment.ID)
		}
		if utf8.RuneCountInString(text) > maxCommentSnippetChars {
			// Truncate the AI-returned snippet. applyCommentText will replace
			// this with the original HN comment text after validation anyway.
			runes := []rune(text)
			comment.Text = string(runes[:maxCommentSnippetChars])
		}
		if len(comment.Indicators) == 0 {
			return fmt.Errorf("comment id %d is missing indicators", comment.ID)
		}
		for i, indicator := range comment.Indicators {
			normalized := strings.ToLower(strings.TrimSpace(indicator))
			if _, ok := allowedIndicators[normalized]; !ok {
				return fmt.Errorf("comment id %d has invalid indicator %q", comment.ID, indicator)
			}
			comment.Indicators[i] = normalized
		}
		if comment.AccuracyRank < 1 || comment.AccuracyRank > len(expectedIDs) {
			return fmt.Errorf("comment id %d has accuracyRank %d out of range", comment.ID, comment.AccuracyRank)
		}
		if _, ok := ranks[comment.AccuracyRank]; ok {
			return fmt.Errorf("accuracyRank %d is duplicated", comment.AccuracyRank)
		}
		ranks[comment.AccuracyRank] = struct{}{}
		if strings.TrimSpace(comment.Analysis) == "" {
			return fmt.Errorf("comment id %d has empty analysis", comment.ID)
		}
	}
	if len(seenIDs) != len(expectedIDs) {
		return fmt.Errorf("comment IDs are incomplete")
	}
	if len(ranks) != len(expectedIDs) {
		return fmt.Errorf("accuracy ranks are incomplete")
	}
	return nil
}
