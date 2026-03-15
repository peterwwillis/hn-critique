// Command crawler fetches top Hacker News stories, generates AI critiques, and
// writes a static website to the docs/ directory for deployment to GitHub Pages.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/article"
	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
	"github.com/peterwwillis/hn-critique/internal/hn"
)

const (
	defaultStoryCount                = 30
	defaultCommentDepth              = 3
	maxTopComments                   = 20
	maxChildComments                 = 5
	articleRetrievalFailureReason    = "the article could not be retrieved"
	articleInsufficientContentReason = "the article did not contain enough readable content to analyze"
	// Pause between HN API calls to be a good citizen.
	hnDelay = 100 * time.Millisecond
	// Pause between full story fetches (article + AI) to avoid rate limiting.
	storyDelay = 2 * time.Second
)

func main() {
	var (
		storyCount   = flag.Int("stories", defaultStoryCount, "number of top stories to fetch")
		outputDir    = flag.String("out", "docs", "output directory for the generated site")
		skipAI       = flag.Bool("skip-ai", false, "skip AI analysis (useful for testing)")
		prepareInput = flag.Bool("prepare-input", false, "fetch stories and cache analysis input, then exit without AI or site generation")
		analyzeInput = flag.Bool("analyze-input", false, "load cached analysis input and run AI analysis/site generation")
		configPath   = flag.String("config", "", "path to TOML config file (default: hn-critique.toml if present)")
		providerFlag = flag.String("provider", "", "AI provider to use: openai, ollama, github (overrides config file)")
		strict       = flag.Bool("strict", false, "exit non-zero if any AI analysis warning or error occurs")
		siteURL      = flag.String("site-url", "https://peterwwillis.github.io/hn-critique", "base URL of the published site, used in the end-of-run incomplete-results summary")
	)
	flag.Parse()

	if *prepareInput && *analyzeInput {
		log.Fatal("cannot use -prepare-input and -analyze-input together")
	}

	// Auto-detect config file if not specified.
	if *configPath == "" {
		if _, err := os.Stat("hn-critique.toml"); err == nil {
			*configPath = "hn-critique.toml"
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Allow -provider flag to override config file setting.
	if *providerFlag != "" {
		cfg.Provider = config.ProviderName(*providerFlag)
	}

	if *prepareInput {
		*skipAI = true
	}
	modelConfig := cfg.SelectedModelConfig()
	limits := modelConfig.Limits

	hnClient := hn.NewClient()
	articleFetcher := article.NewFetcherWithLimits(article.Limits{
		MaxBodyBytes: limits.ArticleBodyBytes,
		MaxTextLen:   limits.ArticleTextChars,
	})

	var aiProvider ai.Provider
	if !*skipAI {
		if err := cfg.Validate(); err != nil {
			log.Printf("AI analysis disabled: %v", err)
			if *strict {
				log.Fatal("strict mode: AI provider is required but could not be configured")
			}
			*skipAI = true
		} else {
			p, err := ai.NewProvider(cfg)
			if err != nil {
				log.Printf("AI analysis disabled: %v", err)
				if *strict {
					log.Fatal("strict mode: AI provider is required but could not be configured")
				}
				*skipAI = true
			} else {
				aiProvider = p
				log.Printf("Using AI provider: %s", aiProvider.Name())
			}
		}
	}

	gen := generator.New(*outputDir)

	// Ensure output directories exist early so the cache is accessible.
	if err := os.MkdirAll(generator.CacheDir(*outputDir), 0o755); err != nil {
		log.Printf("Warning: could not create cache directory: %v", err)
	}

	var analysisWarnings int

	var stories []*generator.Story
	if *analyzeInput {
		log.Printf("Loading cached story inputs…")
		loaded, err := generator.LoadStoryInputs(*outputDir)
		if err != nil {
			log.Fatalf("Failed to load story inputs: %v", err)
		}
		stories = loaded
	} else {
		log.Printf("Fetching top %d stories…", *storyCount)
		storyIDs, err := hnClient.GetTopStories(*storyCount)
		if err != nil {
			log.Fatalf("Failed to fetch top stories: %v", err)
		}

		commentDepth := limits.CommentDepth
		if commentDepth == 0 {
			commentDepth = defaultCommentDepth
		}
		topComments := limits.TopComments
		if topComments == 0 {
			topComments = maxTopComments
		}
		childComments := limits.ChildComments
		if childComments == 0 {
			childComments = maxChildComments
		}

		stories = make([]*generator.Story, 0, len(storyIDs))
		for rank, id := range storyIDs {
			log.Printf("[%d/%d] Processing story %d…", rank+1, len(storyIDs), id)

			item, err := hnClient.GetItem(id)
			if err != nil {
				log.Printf("  ⚠  skip story %d: %v", id, err)
				continue
			}
			if item.Deleted || item.Dead || item.Type != "story" {
				log.Printf("  ⚠  skip story %d: deleted/dead/wrong type", id)
				continue
			}

			story := &generator.Story{
				ID:           item.ID,
				Rank:         rank + 1,
				Title:        item.Title,
				URL:          item.URL,
				Domain:       extractDomain(item.URL),
				Score:        item.Score,
				Author:       item.By,
				Time:         item.Time,
				CommentCount: item.Descendants,
			}

			// Fetch comments.
			if len(item.Kids) > 0 {
				log.Printf("  Fetching comments…")
				story.Comments = fetchComments(hnClient, item.Kids, commentDepth, topComments, childComments)
			}

			// Fetch article content (only for stories with external URLs).
			if item.URL != "" {
				log.Printf("  Fetching article: %s", item.URL)
				text, truncated, err := articleFetcher.Fetch(item.URL)
				if err != nil {
					log.Printf("  ⚠  article fetch failed: %v", err)
					story.ArticleUnavailableReason = articleRetrievalFailureReason
				} else if strings.TrimSpace(text) == "" {
					story.ArticleUnavailableReason = articleInsufficientContentReason
				} else {
					story.ArticleText = text
					if truncated {
						story.ArticleTruncated = true
						runeCount := utf8.RuneCountInString(text)
						log.Printf("  ⚠  article text truncated: fetched %d chars (limit: %d); critique may be incomplete",
							runeCount, limits.ArticleTextChars)
					}
					// Check whether the text will be further truncated when
					// inserted into the AI prompt.
					if limits.ArticlePromptBytes > 0 && len(text) > limits.ArticlePromptBytes {
						story.ArticleTruncated = true
						log.Printf("  ⚠  article prompt truncated: text is %d bytes, limit is %d; critique may be incomplete",
							len(text), limits.ArticlePromptBytes)
					}
				}
			}

			stories = append(stories, story)
			if *prepareInput {
				time.Sleep(storyDelay)
			}
		}
	}

	if *prepareInput {
		if err := generator.SaveStoryInputs(*outputDir, stories); err != nil {
			log.Fatalf("Failed to save story inputs: %v", err)
		}
		log.Printf("Saved story inputs to %s", generator.StoryInputPath(*outputDir))
		return
	}

	for _, story := range stories {
		if aiProvider != nil {
			// Load any previously cached analysis as a fallback.
			cached, cacheErr := generator.LoadCache(*outputDir, story.ID, story.Time)
			if cacheErr != nil {
				log.Printf("  ⚠  cache load failed: %v", cacheErr)
			}

			// Article critique.
			if story.ArticleUnavailableReason != "" {
				story.Critique = unavailableCritiqueForReason(story.ArticleUnavailableReason)
			} else if story.ArticleText != "" || story.URL != "" {
				log.Printf("  Analyzing article…")
				crit, err := aiProvider.AnalyzeArticle(story.Title, story.URL, story.ArticleText)
				if err != nil {
					log.Printf("  ⚠  article analysis failed: %v", err)
					analysisWarnings++
					analysisReason := "the AI assessment could not be completed because the AI provider returned an error. The analysis may be retried on the next run"
					story.Critique = unavailableCritiqueForReason(analysisReason)
				} else {
					story.Critique = crit
				}
			}

			// Comments critique.
			if len(story.Comments) > 0 {
				// Warn if comment content will be truncated before sending to the AI.
				if limits.CommentPromptBytes > 0 {
					if total := totalCommentBytes(story.Comments); total > limits.CommentPromptBytes {
						story.CommentsTruncated = true
						log.Printf("  ⚠  comment prompt truncated: total comment text ~%d bytes exceeds limit of %d; comments critique may be incomplete",
							total, limits.CommentPromptBytes)
					}
				}
				log.Printf("  Analyzing comments…")
				cc, err := aiProvider.AnalyzeComments(story.Title, story.URL, story.Comments)
				if err != nil {
					log.Printf("  ⚠  comments analysis failed: %v", err)
					analysisWarnings++
					if cached != nil && cached.CommentsCritique != nil {
						log.Printf("  Using cached comments analysis.")
						story.CommentsCritique = cached.CommentsCritique
					}
				} else {
					story.CommentsCritique = cc
				}
			} else if cached != nil && cached.CommentsCritique != nil {
				story.CommentsCritique = cached.CommentsCritique
			}

			// Save updated analysis to cache.
			if story.Critique != nil || story.CommentsCritique != nil {
				newCache := &generator.AnalysisCache{
					StoryID:          story.ID,
					Critique:         story.Critique,
					CommentsCritique: story.CommentsCritique,
				}
				if saveErr := generator.SaveCache(*outputDir, story.ID, story.Time, newCache); saveErr != nil {
					log.Printf("  ⚠  cache save failed: %v", saveErr)
				}
			}
		}

		shouldUsePlaceholder := aiProvider == nil && !*prepareInput && story.URL != "" && story.Critique == nil
		if shouldUsePlaceholder {
			if story.ArticleUnavailableReason != "" {
				story.Critique = unavailableCritiqueForReason(story.ArticleUnavailableReason)
			} else if story.ArticleText != "" {
				analysisReason := "the AI assessment is not available"
				story.Critique = unavailableCritiqueForReason(analysisReason)
			}
		}

		if !*analyzeInput {
			time.Sleep(storyDelay)
		}
	}

	log.Printf("Generating static site in %s/ (%d stories)…", *outputDir, len(stories))
	if err := gen.Generate(stories); err != nil {
		log.Fatalf("Site generation failed: %v", err)
	}
	log.Println("Done.")

	// Print a summary of stories whose analysis may be incomplete due to truncation.
	printIncompleteResultsSummary(stories, strings.TrimRight(*siteURL, "/"))

	if *strict && analysisWarnings > 0 {
		log.Fatalf("strict mode: %d analysis warning(s) encountered", analysisWarnings)
	}
}

// fetchComments recursively fetches comments up to depth levels deep,
// stopping after maxCount top-level comments.
func fetchComments(client *hn.Client, kids []int, depth, maxCount, maxChildCount int) []*generator.Comment {
	return fetchCommentsAtDepth(client, kids, depth, maxCount, maxChildCount, 0)
}

func fetchCommentsAtDepth(client *hn.Client, kids []int, depth, maxCount, maxChildCount, currentDepth int) []*generator.Comment {
	if depth == 0 || len(kids) == 0 {
		return nil
	}

	var comments []*generator.Comment
	for _, id := range kids {
		if len(comments) >= maxCount {
			break
		}

		item, err := client.GetItem(id)
		if err != nil || item == nil || item.Deleted || item.Dead || item.Type != "comment" {
			time.Sleep(hnDelay)
			continue
		}

		comment := &generator.Comment{
			ID:     item.ID,
			Author: item.By,
			Text:   template.HTML(item.Text), //nolint:gosec // HN sanitizes comment HTML
			Time:   item.Time,
			Depth:  currentDepth,
		}

		if len(item.Kids) > 0 {
			comment.Kids = fetchCommentsAtDepth(client, item.Kids, depth-1, maxChildCount, maxChildCount, currentDepth+1)
		}

		comments = append(comments, comment)
		time.Sleep(hnDelay)
	}
	return comments
}

// extractDomain returns the bare hostname from a URL, stripping the www. prefix.
func extractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	host = strings.TrimPrefix(host, "www.")
	return host
}

func unavailableCritique(summary, truthfulness string) *generator.ArticleCritique {
	return &generator.ArticleCritique{
		Summary:      summary,
		Truthfulness: truthfulness,
		Rating:       "unavailable",
	}
}

func unavailableSummary(reason string) string {
	return "Summary unavailable because " + reason + "."
}

func unavailableTruthfulness(reason string) string {
	return "Truthfulness assessment unavailable because " + reason + "."
}

func unavailableCritiqueForReason(reason string) *generator.ArticleCritique {
	return unavailableCritique(unavailableSummary(reason), unavailableTruthfulness(reason))
}

// totalCommentBytes returns the approximate byte count of formatted comment
// text that would be sent to the AI, used to predict prompt truncation.
func totalCommentBytes(comments []*generator.Comment) int {
	var total int
	for _, c := range comments {
		total += len(fmt.Sprintf("[id:%d by:%s]\n%s\n\n", c.ID, c.Author, c.Text))
	}
	return total
}

// printIncompleteResultsSummary logs a human-readable summary of stories that
// may have incomplete or less accurate analysis due to content truncation.
func printIncompleteResultsSummary(stories []*generator.Story, baseURL string) {
	var incomplete []*generator.Story
	for _, s := range stories {
		if s.ArticleTruncated || s.CommentsTruncated {
			incomplete = append(incomplete, s)
		}
	}
	if len(incomplete) == 0 {
		return
	}

	log.Printf("=== Incomplete Results Summary ===")
	noun := "stories"
	if len(incomplete) == 1 {
		noun = "story"
	}
	log.Printf("%d %s may have incomplete analysis due to content truncation.", len(incomplete), noun)
	log.Printf("Review the following pages for potential inaccuracies:")
	for _, s := range incomplete {
		var reasons []string
		if s.ArticleTruncated {
			reasons = append(reasons, "article content truncated")
		}
		if s.CommentsTruncated {
			reasons = append(reasons, "comments content truncated")
		}
		log.Printf("  [%d] %q (%s)", s.Rank, s.Title, s.Domain)
		log.Printf("      Reasons: %s", strings.Join(reasons, ", "))
		if baseURL != "" && s.CritiquePath != "" {
			log.Printf("      Critique: %s/%s", baseURL, s.CritiquePath)
		}
		if baseURL != "" && s.CommentsPath != "" {
			log.Printf("      Comments: %s/%s", baseURL, s.CommentsPath)
		}
	}
}
