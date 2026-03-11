# hn-critique

A version of Hacker News that uses AI to critique the articles and comments.

The site mimics the Hacker News front page. Next to each story's "comments" link there is a **critique** link that leads to:

- **Article critique** — an AI assessment of whether the article is truthful, a summary of its main points, and additional considerations not mentioned in the piece.
- **Comments critique** — a summary of the comment thread, with each comment ranked from most to least accurate and annotated with short indicators (e.g. *thoughtful*, *emotional*, *constructive*, *trolling*).

All pages are pre-rendered static HTML, suitable for caching by Cloudflare or a browser.

## How it works

1. A [GitHub Actions workflow](.github/workflows/crawl.yml) runs hourly.
2. The Go backend (`cmd/crawler`) fetches the top 30 Hacker News stories via the official [HN Firebase API](https://github.com/HackerNews/API).
3. For each story it fetches the full article text, falling back to [archive.ph](https://archive.ph) and the [Wayback Machine](https://web.archive.org) if the page is paywalled.
4. OpenAI's **Responses API** (with the `web_search_preview` tool) analyzes the article for truthfulness.  
   OpenAI's **Chat Completions API** (JSON mode) analyzes the comment section.
5. Static HTML is written to `docs/` and deployed to GitHub Pages via [`peaceiris/actions-gh-pages`](https://github.com/peaceiris/actions-gh-pages).

## Setup

1. Fork this repository.
2. Add an **`OPENAI_API_KEY`** secret in *Settings → Secrets and variables → Actions*.
3. Enable **GitHub Pages** in *Settings → Pages* and set the source branch to `gh-pages`.
4. The workflow runs automatically every hour, or you can trigger it manually from the *Actions* tab.

## Local development

```bash
# Build
go build ./...

# Run tests
go test ./...

# Generate a site without AI analysis (useful for UI iteration)
go run ./cmd/crawler/ -skip-ai -stories 10 -out ./docs
```

The `-skip-ai` flag skips OpenAI calls and generates placeholder pages — no API key required.

## Project layout

```
cmd/crawler/          Main entry point
internal/hn/          Hacker News API client
internal/article/     Article fetcher with paywall bypass
internal/ai/          OpenAI integration (article + comments analysis)
internal/generator/   Static HTML generator
  templates/          HTML templates (HN-like look)
  static/             CSS stylesheet
.github/workflows/    Hourly GitHub Actions crawl + deploy
```
