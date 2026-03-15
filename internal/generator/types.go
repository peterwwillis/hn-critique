// Package generator defines shared data types used across the HN Critique application.
package generator

import "html/template"

// Story holds all data for a single HN story, including fetched article text and AI critiques.
type Story struct {
	ID               int
	Rank             int
	Title            string
	URL              string
	Domain           string
	Score            int
	Author           string
	Time             int64
	CommentCount     int
	Comments         []*Comment
	ArticleText      string
	Critique         *ArticleCritique
	CommentsCritique *CommentsCritique
	CritiquePath     string
	CommentsPath     string
}

// Comment represents a single HN comment with its nested replies.
type Comment struct {
	ID     int
	Author string
	// Text is the HTML body of the comment as returned by the HN API.
	Text template.HTML
	Time int64
	// Depth is the nesting level: 0 for top-level, 1 for first reply, etc.
	Depth int
	Kids  []*Comment
}

// ArticleCritique is the AI-generated analysis of an article.
type ArticleCritique struct {
	Summary        string   `json:"summary"`
	MainPoints     []string `json:"mainPoints"`
	Truthfulness   string   `json:"truthfulness"`
	Considerations []string `json:"considerations"`
	// Rating is one of: reliable, questionable, misleading, unavailable.
	Rating string `json:"rating"`
}

// CommentsCritique is the AI-generated analysis of a story's comment section.
type CommentsCritique struct {
	Summary  string            `json:"summary"`
	Comments []AnalyzedComment `json:"comments"`
}

// AnalyzedComment is a top-level comment annotated with AI-generated indicators.
type AnalyzedComment struct {
	ID     int    `json:"id"`
	Author string `json:"author"`
	// Text is the plain-text snippet shown next to the critique.
	Text string `json:"text"`
	// Indicators are short descriptors such as "thoughtful", "trolling", etc.
	Indicators []string `json:"indicators"`
	// AccuracyRank is the position when sorted from most (1) to least accurate.
	AccuracyRank int `json:"accuracyRank"`
	// Analysis is a one- or two-sentence AI assessment.
	Analysis string `json:"analysis"`
}
