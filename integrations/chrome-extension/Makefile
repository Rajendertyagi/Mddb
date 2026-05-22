# Makefile for integrations/chrome-extension.
# Run from this directory or via the top-level Makefile (make chrome-build, ...).

.PHONY: help install build package test test-coverage lint format check audit clean

NODE_BIN ?= npx

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install npm dependencies
	npm ci || npm install

build: ## Bundle popup/options/background into ./build via esbuild
	npm run build

package: build ## Build and zip into dist/mddb-browser-<version>.zip
	npm run package

test: ## Run Jest unit tests
	$(NODE_BIN) jest

test-coverage: ## Run tests with coverage (enforces 90%)
	$(NODE_BIN) jest --coverage

lint: ## ESLint
	npm run lint

format: ## Prettier (write)
	npm run format

audit: ## npm audit (production deps only, >=high severity)
	npm run audit

check: ## Format check + lint + test + build + package (matches CI all)
	npm run all

clean: ## Remove build artefacts and dependencies
	rm -rf node_modules build dist coverage *.zip
