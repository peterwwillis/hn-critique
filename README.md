# hn-critique

View the Hacker News top stories with an AI critic.

The site mimics the Hacker News front page. Next to each story's "comments" link there is a **critique** link that leads to:

- **Article critique** — an AI assessment of whether the article is truthful, a summary of its main points, and additional considerations not mentioned in the piece.
- **Comments critique** — a summary of the comment thread, with each comment ranked from most to least accurate and annotated with short indicators (e.g. *thoughtful*, *emotional*, *constructive*, *trolling*).

## Disclaimer

This project is an experimental tool that uses Large Language Models (LLMs) to generate automated critiques and ratings. The outputs are provided for informational and entertainment purposes only. They do not represent the views of the author, nor are they a substitute for human judgment or professional analysis. The AI may "hallucinate" or provide inaccurate, biased, or misleading information. Do not rely on these ratings for any decision-making or critical evaluation.

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
| `OPENAI_CHAT_MODEL` | Model name for article and comment analysis | `gpt-4o-mini` |

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

If you run a self-hosted GitHub Actions runner on a machine that has access to
a private network (e.g. an internal Ollama or llama-server instance), the
crawler can reach it using environment variables — no API key or config file
required.

**Quick overview:**

1. Install and start your inference server on the runner machine (or on a LAN
   machine the runner can reach).
2. Register a [self-hosted runner](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/adding-self-hosted-runners)
   and give it a label (e.g. `self-hosted-ai`).
3. Set repository (or [GitHub Environment](https://docs.github.com/en/actions/deployment/targeting-different-environments/using-environments-for-deployment))
   variables:

   | Variable | Example value |
   |---|---|
   | `OPENAI_BASE_URL` | `http://localhost:11434` (or `http://192.168.1.50:11434` for a LAN server) |
   | `OPENAI_CHAT_MODEL` | `llama3.2` (or whichever model your server loads) |

4. In `.github/workflows/crawl.yml`, set `runs-on` to your runner label:

   ```yaml
   jobs:
     crawl-and-deploy:
       runs-on: self-hosted-ai
   ```

   The workflow already passes `OPENAI_BASE_URL` and `OPENAI_CHAT_MODEL` from
   repository/environment variables to the crawler — no further changes needed.

For a complete step-by-step guide covering both topologies (AI on the runner
machine vs. AI on a separate LAN machine), GitHub Environments scoping, a
copy-paste workflow file, and troubleshooting tips, see
**[docs/self-hosted-runner.md](docs/self-hosted-runner.md)**.

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
