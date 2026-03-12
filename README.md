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
4. An AI provider analyses the article for truthfulness and critiques the comment section (see [AI providers](#ai-providers) below).
5. Static HTML is written to `docs/` and deployed to GitHub Pages via [`peaceiris/actions-gh-pages`](https://github.com/peaceiris/actions-gh-pages).

## Setup

1. Fork this repository.
2. Choose an AI provider (see below) and add the required secret(s).
3. Enable **GitHub Pages** in *Settings → Pages* and set the source branch to `gh-pages`.
4. The workflow runs automatically every hour, or you can trigger it manually from the *Actions* tab.

## AI providers

The crawler supports three providers. All share the same `hn-critique.toml` config file (see [`hn-critique.toml.example`](hn-critique.toml.example)) and can be selected with the `-provider` flag or the `provider` key in the config.

### OpenAI (default)

Uses [api.openai.com](https://platform.openai.com/). Requires an API key.

**GitHub Actions setup:**

```yaml
# Settings → Secrets and variables → Actions → New repository secret
# Name: OPENAI_API_KEY
# Value: sk-…
```

```yaml
# In your workflow step:
env:
  OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

**Configurable via environment variable:**

| Variable | Description | Default |
|---|---|---|
| `OPENAI_API_KEY` | API key for api.openai.com | *(required unless `OPENAI_BASE_URL` is set)* |
| `OPENAI_BASE_URL` | Root URL of any OpenAI-compatible server | `https://api.openai.com` |

### OpenAI-compatible local/private backends (Ollama, llama-server, …)

The `openai` provider can be pointed at **any OpenAI-compatible server** by setting `base_url` in the config or the `OPENAI_BASE_URL` environment variable. No API key is required for most local backends.

| Backend | `base_url` |
|---|---|
| [Ollama](https://ollama.com/) | `http://localhost:11434` |
| [llama-server](https://github.com/ggml-org/llama.cpp) (llama.cpp) | `http://localhost:8080` |
| [LM Studio](https://lmstudio.ai/) | `http://localhost:1234` |
| [vLLM](https://docs.vllm.ai/) | `http://localhost:8000` |

**Using Ollama via `hn-critique.toml`:**

```toml
provider = "openai"

[openai]
base_url  = "http://localhost:11434"
chat_model = "llama3.2"
use_responses_api = false
```

**Using Ollama via environment variables (no config file needed):**

```bash
OPENAI_BASE_URL=http://localhost:11434 \
go run ./cmd/crawler/ -provider openai
```

> **Backward compatibility:** `provider = "ollama"` with a `[ollama]` section continues to work. The `ollama` provider now routes through the same OpenAI-compatible client as the `openai` provider, so there is no behavioural difference. Prefer the `openai` provider with `base_url` for new setups.

### GitHub Models

Uses the [GitHub Models inference API](https://docs.github.com/en/github-models). No external account is needed when running inside a GitHub Actions workflow — just add `permissions: models: read` to the workflow job.

**GitHub Actions setup (no secrets needed):**

```yaml
jobs:
  crawl-and-deploy:
    permissions:
      models: read
    steps:
      - run: go run ./cmd/crawler/ -provider github
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

First enable GitHub Models in your repository: *Settings → Copilot → Use GitHub Models*.

## Using a self-hosted runner with a local AI server

If you run a self-hosted GitHub Actions runner on a machine that has access to a private network (e.g. an internal Ollama or llama-server instance), the crawler can reach it using the `OPENAI_BASE_URL` environment variable — no API key required.

### 1 — Register a self-hosted runner

Follow the [GitHub docs](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/adding-self-hosted-runners) to register a runner on your server. Give it a label (e.g. `self-hosted-ai`) so the workflow can target it.

### 2 — Start your local AI server on the runner machine

```bash
# Example: Ollama
ollama serve           # listens on http://localhost:11434 by default
ollama pull llama3.2   # pull the model you want to use
```

### 3 — Add `OPENAI_BASE_URL` as a repository variable

Go to *Settings → Secrets and variables → Actions → Variables → New repository variable*:

| Name | Value |
|---|---|
| `OPENAI_BASE_URL` | `http://localhost:11434` |

### 4 — Point the workflow at the self-hosted runner

In `.github/workflows/crawl.yml`, change `runs-on` to your runner label:

```yaml
jobs:
  crawl-and-deploy:
    runs-on: self-hosted-ai   # ← your runner label
    steps:
      - run: go run ./cmd/crawler/ -provider openai
        env:
          OPENAI_BASE_URL: ${{ vars.OPENAI_BASE_URL }}
```

Because the crawler reads `OPENAI_BASE_URL` at runtime, no config file change is needed — the same workflow works with `ubuntu-latest` (cloud) or a self-hosted runner just by toggling the `OPENAI_BASE_URL` variable.

## Local development

```bash
# Build
go build ./...

# Run tests
go test ./...

# Generate a site without AI analysis (useful for UI iteration)
go run ./cmd/crawler/ -skip-ai -stories 10 -out ./docs

# Generate using a local Ollama instance
OPENAI_BASE_URL=http://localhost:11434 \
go run ./cmd/crawler/ -provider openai -stories 5 -out ./docs
```

The `-skip-ai` flag skips all AI calls and generates placeholder pages — no API key or local server required.

## Project layout

```
cmd/crawler/          Main entry point
internal/hn/          Hacker News API client
internal/article/     Article fetcher with paywall bypass
internal/ai/          AI provider abstraction (OpenAI-compatible + GitHub Models)
internal/generator/   Static HTML generator
  templates/          HTML templates (HN-like look)
  static/             CSS stylesheet
.github/workflows/    Hourly GitHub Actions crawl + deploy
hn-critique.toml.example  Annotated configuration reference
```
