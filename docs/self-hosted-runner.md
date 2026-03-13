# Using a self-hosted GitHub Actions runner with a local AI server

This guide explains how to run hn-critique's hourly crawl on a **self-hosted
GitHub Actions runner** that has access to a local or LAN-accessible
OpenAI-compatible inference server (Ollama, llama-server, LM Studio, vLLM,
…).

The key advantage over the default cloud runner (`ubuntu-latest`) is that your
runner machine can reach a private inference server that is not exposed to the
public internet — no OpenAI API key or external account is required.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| A machine you control | Linux, macOS, or Windows — any OS supported by the GitHub Actions runner |
| Go ≥ the version in `go.mod` | Install from [go.dev/dl](https://go.dev/dl/) or via your package manager |
| An OpenAI-compatible inference server | See [supported backends](#supported-backends) below |
| GitHub repository with Actions enabled | Fork this repository or use your own |

---

## Supported backends

The crawler works with any server that implements the OpenAI
`/v1/chat/completions` endpoint.

| Backend | Default URL | Install guide |
|---|---|---|
| [Ollama](https://ollama.com/) | `http://localhost:11434` | `curl -fsSL https://ollama.com/install.sh \| sh` |
| [llama-server](https://github.com/ggml-org/llama.cpp) (llama.cpp) | `http://localhost:8080` | [build from source](https://github.com/ggml-org/llama.cpp#build) |
| [LM Studio](https://lmstudio.ai/) | `http://localhost:1234` | Download from lmstudio.ai |
| [vLLM](https://docs.vllm.ai/) | `http://localhost:8000` | `pip install vllm` |

---

## Step 1 — Install and start your inference server

The examples below use **Ollama**. Substitute the equivalent commands for
your chosen backend.

```bash
# Install Ollama (Linux / macOS)
curl -fsSL https://ollama.com/install.sh | sh

# Pull the model you want to use
ollama pull llama3.2

# Start the server (runs on http://localhost:11434 by default)
ollama serve
```

Verify the server is reachable:

```bash
curl http://localhost:11434/v1/models
```

You should see a JSON list of available models.

---

## Step 2 — Register a self-hosted runner

Follow the [GitHub documentation](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/adding-self-hosted-runners)
to register the runner on your machine.

1. Go to your repository → **Settings → Actions → Runners → New self-hosted runner**.
2. Follow the on-screen instructions to download and configure the runner agent.
3. Give the runner a descriptive **label** (e.g. `self-hosted-ai`). You will
   reference this label in the workflow file.

```bash
# Example runner registration (Linux x64 — copy the exact command from GitHub)
mkdir actions-runner && cd actions-runner
curl -o actions-runner-linux-x64-2.x.x.tar.gz -L https://github.com/actions/runner/releases/download/...
tar xzf ./actions-runner-linux-x64-2.x.x.tar.gz
./config.sh --url https://github.com/YOUR-ORG/hn-critique --token YOUR-TOKEN --labels self-hosted-ai
./run.sh   # start the runner agent (or install as a service)
```

---

## Step 3 — Configure environment variables

The crawler reads three environment variables to locate the inference server
and choose a model. Set them in GitHub without touching the workflow file.

### Option A — Repository variables (applies to all workflows)

Go to **Settings → Secrets and variables → Actions → Variables**:

| Variable | Example value | Purpose |
|---|---|---|
| `OPENAI_BASE_URL` | `http://localhost:11434` | Root URL of your inference server |
| `OPENAI_CHAT_MODEL` | `llama3.2` | Model name served by your backend |

> **Note:** If your inference server runs on a **separate machine** on your
> LAN (not on the runner itself), replace `localhost` with the server's IP
> address or hostname, e.g. `http://192.168.1.50:11434`.

### Option B — GitHub Environment (recommended — scopes variables to a specific runner)

Using a **GitHub Environment** prevents the local backend variables from
accidentally being used by cloud runners.

1. Go to **Settings → Environments → New environment** and name it
   `local-ai`.
2. Add the variables from the table above to the environment.
3. Optionally, configure **deployment protection rules** to restrict which
   branches can trigger deployments to this environment.

See [Using environments for deployment](https://docs.github.com/en/actions/deployment/targeting-different-environments/using-environments-for-deployment)
for full details.

---

## Step 4 — Customise the workflow

### 4a — Simple approach: edit `crawl.yml` in place

Change `runs-on` to your runner label and, if you used Option B above, add
`environment: local-ai`:

```yaml
jobs:
  crawl-and-deploy:
    runs-on: self-hosted-ai   # ← your runner label
    environment: local-ai     # ← optional: use a GitHub Environment
```

The existing `crawl.yml` already passes `OPENAI_BASE_URL` and
`OPENAI_CHAT_MODEL` to the crawler step, so no further changes are needed.

### 4b — Parallel approach: separate workflow for the self-hosted runner

If you want to keep the default cloud workflow unchanged and add a separate
self-hosted workflow, create `.github/workflows/crawl-local.yml`:

```yaml
name: Crawl HN and Deploy (self-hosted / local AI)

on:
  schedule:
    - cron: '0 * * * *'
  workflow_dispatch:

permissions:
  contents: write
  pages: write
  id-token: write

jobs:
  crawl-and-deploy:
    runs-on: self-hosted-ai     # label assigned to your runner
    environment: local-ai       # GitHub Environment with OPENAI_BASE_URL /
                                # OPENAI_CHAT_MODEL variables (see Step 3)
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Download dependencies
        run: go mod download

      - name: Run crawler
        env:
          # These are set as variables in the "local-ai" GitHub Environment.
          # OPENAI_BASE_URL  — root URL of your local inference server
          # OPENAI_CHAT_MODEL — model name (e.g. llama3.2, mistral)
          OPENAI_BASE_URL: ${{ vars.OPENAI_BASE_URL }}
          OPENAI_CHAT_MODEL: ${{ vars.OPENAI_CHAT_MODEL }}
        run: go run ./cmd/crawler/... -provider openai

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v4
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs
          publish_branch: gh-pages
          allow_empty_commit: false
          commit_message: 'chore: update HN Critique site (local AI) [${{ github.run_number }}]'
```

---

## Environment variable reference

All variables can be set as **GitHub repository variables** (visible to all
workflows) or scoped to a **GitHub Environment** for finer control.

| Variable | Description | Required? |
|---|---|---|
| `OPENAI_BASE_URL` | Root URL of any OpenAI-compatible server. When unset, defaults to `https://api.openai.com`. | Required for local backends |
| `OPENAI_CHAT_MODEL` | Model name to use for article and comment analysis. When set, overrides both the default (`gpt-4o-mini`) and any value in `hn-critique.toml`. | Required for local backends |
| `OPENAI_API_KEY` | API key for `api.openai.com`. Not needed for local backends. | Required for api.openai.com only |

---

## Network topology

### Topology A — AI server runs on the runner machine (localhost)

```
GitHub Actions runner
  ├── actions-runner agent
  ├── Go crawler (go run ./cmd/crawler/...)
  └── Ollama / llama-server (http://localhost:11434)
```

Set `OPENAI_BASE_URL=http://localhost:11434`.

### Topology B — AI server on a separate LAN machine

```
GitHub Actions runner          LAN AI server (e.g. 192.168.1.50)
  ├── actions-runner agent       └── Ollama (http://0.0.0.0:11434)
  └── Go crawler  ─────────────────────────────────────────────►
```

Set `OPENAI_BASE_URL=http://192.168.1.50:11434`.

Make sure the inference server is configured to listen on all interfaces (not
just localhost). For Ollama:

```bash
# Allow connections from other machines
OLLAMA_HOST=0.0.0.0 ollama serve
```

---

## Troubleshooting

### "connection refused" when the crawler tries to reach the AI server

- Confirm the server is running: `curl http://SERVERIP:PORT/v1/models`
- For Topology B, check firewall rules on the AI server machine.
- Ensure `OPENAI_BASE_URL` uses the correct IP/hostname (not `localhost` for
  Topology B).

### Model not found / "unknown model" error

- Run `ollama list` (or your backend's equivalent) to see loaded models.
- Make sure `OPENAI_CHAT_MODEL` matches the exact model name the server uses
  (e.g. `llama3.2`, not `llama3.2:latest`).

### Go not found on the runner

Install Go on the runner machine, or let the workflow install it automatically
via the `actions/setup-go` step (included in the example above).

### The cloud runner (`ubuntu-latest`) fails because `OPENAI_BASE_URL` is set to a LAN address

Use **Option B (GitHub Environments)** in Step 3 so that `OPENAI_BASE_URL` is
only available to jobs that target the `local-ai` environment. Cloud runner
jobs that do not specify `environment: local-ai` will not inherit the variable.
