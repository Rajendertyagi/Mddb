# Makefile for integrations/github-action.
# Run from this directory or via the top-level Makefile (make gha-build, ...).

.PHONY: help install build test test-coverage lint format check verify-dist clean

NODE_BIN ?= npx

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install npm dependencies
	npm ci || npm install

build: ## Bundle the action into dist/index.js
	npm run build

test: ## Run Jest unit tests
	$(NODE_BIN) jest

test-coverage: ## Run tests with coverage (enforces 90%)
	$(NODE_BIN) jest --coverage

lint: ## ESLint
	npm run lint

format: ## Prettier (write)
	npm run format

check: ## Format check + lint + test + build (matches CI all)
	npm run all

verify-dist: build ## Build and assert dist/ has no uncommitted changes
	@if ! git diff --exit-code -- dist/ > /dev/null; then \
		echo "::error::dist/ is out of date. Run 'make build' and commit the result."; \
		git --no-pager diff --stat dist/; \
		exit 1; \
	fi

clean: ## Remove build artefacts and dependencies
	rm -rf node_modules dist/index.js dist/index.js.map dist/licenses.txt dist/sourcemap-register.js coverage build
