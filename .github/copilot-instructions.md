# Copilot instructions for hn-critique

## Repository overview
- Go module. Main entry point: `cmd/crawler`.
- Core packages:
  - `internal/hn`: Hacker News API client
  - `internal/article`: article fetch + paywall fallback
  - `internal/ai`: AI provider abstraction (OpenAI-compatible + GitHub Models)
  - `internal/generator`: static HTML generator (templates + CSS)
- Generated site output lives in `docs/` (published to GitHub Pages).
- Config reference: `hn-critique.toml.example` (copy to `hn-critique.toml` for local runs).

## Common commands (prefer Makefile targets)
- `make build` — compile crawler to `./bin/crawler`
- `make vet` — `go vet ./...`
- `make test` — unit tests (fast, no network)
- `make test-integration` — integration tests (requires network)
- `make test-ai` — AI integration tests (needs `GITHUB_TOKEN` or `OPENAI_API_KEY`)
- `make test-all` — unit + integration tests

## Running locally
- Generate a site without AI: `go run ./cmd/crawler/ -skip-ai -stories 10 -out ./docs`
- Use a local OpenAI-compatible backend (e.g., Ollama):
  - `OPENAI_BASE_URL=http://localhost:11434 go run ./cmd/crawler/ -provider openai -stories 5 -out ./docs`
- Config/env vars of note:
  - `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_CHAT_MODEL`
  - `GITHUB_TOKEN` for GitHub Models provider

## Workflow/CI context
- `.github/workflows/crawl.yml` runs hourly to generate and deploy the site.
- GitHub Pages deploy pushes to the `gh-pages` branch via `peaceiris/actions-gh-pages`.

## Known errors encountered & workarounds
- **GH013 repository rule violation when pushing** (observed in the `Crawl HN and Deploy` workflow and in a Copilot workflow).
  - Error: `remote: error: GH013: Repository rule violations found... Changes must be made through a pull request.`
  - Workaround: update repository rules to allow the workflow’s `GITHUB_TOKEN` (or the `gh-pages` branch) to bypass the PR-only requirement, or switch deployment to a PR-based flow. For Copilot updates, ensure changes are pushed through a PR branch (as this agent does via `report_progress`).

## Mergeability check (required before closing tasks)
When completing a series of operations (especially after resolving conflicts or rebasing), always verify mergeability before closing the task:

1. Fetch main and ensure history is complete: `git fetch origin main:refs/remotes/origin/main` (use `git fetch --unshallow origin` if needed).
2. Check for conflicts against main:
   - `git merge-base HEAD origin/main`
   - `git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main`
3. Scan for conflict markers in the working tree: `rg '(<{7}|={7}|>{7})'`.
4. If conflicts are found, resolve them, re-run the checks above, and only then finalize.
