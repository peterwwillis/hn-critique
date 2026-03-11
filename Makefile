# HN Critique — development automation
#
# Usage:
#   make build            Compile the crawler binary
#   make test             Run unit tests (fast, no network)
#   make test-integration Run integration tests (requires network)
#   make test-ai          Run AI integration tests (requires GITHUB_TOKEN or OPENAI_API_KEY)
#   make test-all         Run all tests (unit + integration)
#   make vet              Run static analysis
#   make clean            Remove build artifacts

.DEFAULT_GOAL := help

BINARY     := ./bin/crawler
TEST_FLAGS ?= -timeout 120s -count=1

.PHONY: help build vet test test-unit test-integration test-ai test-all clean

help: ## Show this help message
	@awk 'BEGIN{FS=":.*?## "}/^[a-zA-Z_-]+:.*?## /{printf "  %-22s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

build: ## Compile the crawler binary to ./bin/crawler
	mkdir -p bin
	go build -o $(BINARY) ./cmd/crawler/

vet: ## Run go vet static analysis
	go vet ./...

test: test-unit ## Alias for test-unit

test-unit: ## Fast unit tests — no network or external services required
	go test $(TEST_FLAGS) ./...

test-integration: ## Integration tests — requires live network access
	go test -tags integration $(TEST_FLAGS) ./...

test-ai: ## AI integration tests — requires GITHUB_TOKEN (GitHub Models) or OPENAI_API_KEY
	go test -tags integration -run ^TestAI $(TEST_FLAGS) ./...

test-all: test-unit test-integration ## Run unit tests then integration tests

clean: ## Remove build artifacts (bin/)
	rm -rf bin/
