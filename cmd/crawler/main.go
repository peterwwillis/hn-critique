// Command crawler fetches top Hacker News stories, generates AI critiques, and
// writes a static website to the docs/ directory for deployment to GitHub Pages.
package main

import (
	"flag"
	"html/template"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/article"
	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
	"github.com/peterwwillis/hn-critique/internal/hn"
)

const (
	defaultStoryCount   = 30
	defaultCommentDepth = 3
	maxTopComments      = 20
	maxChildComments    = 5
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
		configPath   = flag.String("config", "", "path to TOML config file (default: hn-critique.toml if present)")
		providerFlag = flag.String("provider", "", "AI provider to use: openai, ollama, github (overrides config file)")
	)
	flag.Parse()

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

	hnClient := hn.NewClient()
	articleFetcher := article.NewFetcher()

	var aiProvider ai.Provider
	if !*skipAI {
		if err := cfg.Validate(); err != nil {
			log.Printf("AI analysis disabled: %v", err)
			*skipAI = true
		} else {
			p, err := ai.NewProvider(cfg)
			if err != nil {
				log.Printf("AI analysis disabled: %v", err)
				*skipAI = true
			} else {
				aiProvider = p
				log.Printf("Using AI provider: %s", aiProvider.Name())
			}
		}
	}

	gen := generator.New(*outputDir)

	log.Printf("Fetching top %d stories…", *storyCount)
	storyIDs, err := hnClient.GetTopStories(*storyCount)
	if err != nil {
		log.Fatalf("Failed to fetch top stories: %v", err)
	}

	stories := make([]*generator.Story, 0, len(storyIDs))

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
			story.Comments = fetchComments(hnClient, item.Kids, defaultCommentDepth, maxTopComments)
		}

		// Fetch article content (only for stories with external URLs).
		if item.URL != "" {
			log.Printf("  Fetching article: %s", item.URL)
			text, err := articleFetcher.Fetch(item.URL)
			if err != nil {
				log.Printf("  ⚠  article fetch failed: %v", err)
			} else {
				story.ArticleText = text
			}
		}

		if aiProvider != nil {
			// Article critique.
			if story.ArticleText != "" || story.URL != "" {
				log.Printf("  Analyzing article…")
				crit, err := aiProvider.AnalyzeArticle(story.Title, story.URL, story.ArticleText)
				if err != nil {
					log.Printf("  ⚠  article analysis failed: %v", err)
				} else {
					story.Critique = crit
				}
			}

			// Comments critique.
			if len(story.Comments) > 0 {
				log.Printf("  Analyzing comments…")
				cc, err := aiProvider.AnalyzeComments(story.Title, story.URL, story.Comments)
				if err != nil {
					log.Printf("  ⚠  comments analysis failed: %v", err)
				} else {
					story.CommentsCritique = cc
				}
			}
		}

		stories = append(stories, story)
		time.Sleep(storyDelay)
	}

	log.Printf("Generating static site in %s/ (%d stories)…", *outputDir, len(stories))
	if err := gen.Generate(stories); err != nil {
		log.Fatalf("Site generation failed: %v", err)
	}
	log.Println("Done.")
}

// fetchComments recursively fetches comments up to depth levels deep,
// stopping after maxCount top-level comments.
func fetchComments(client *hn.Client, kids []int, depth, maxCount int) []*generator.Comment {
	return fetchCommentsAtDepth(client, kids, depth, maxCount, 0)
}

func fetchCommentsAtDepth(client *hn.Client, kids []int, depth, maxCount, currentDepth int) []*generator.Comment {
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
			comment.Kids = fetchCommentsAtDepth(client, item.Kids, depth-1, maxChildComments, currentDepth+1)
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
