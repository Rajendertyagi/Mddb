# Makefile for integrations/grafana-datasource.
# Run from this directory or via the top-level Makefile (make grafana-build, ...).

.PHONY: help install build dev test test-coverage lint format check package docker clean

PLUGIN_ID ?= tradik-mddb-datasource
VERSION   ?= $(shell node -p "require('./package.json').version")

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install npm dependencies
	npm ci || npm install

build: ## Bundle the plugin into dist/
	npm run build

dev: ## Webpack watch mode
	npm run dev

test: ## Run Jest unit tests
	npm test

test-coverage: ## Run tests with coverage (enforces ≥90%)
	npm run test:coverage

lint: ## ESLint
	npm run lint

format: ## Prettier (write)
	npm run format

check: ## Format check + lint + tests + build (matches CI)
	npm run all

package: build ## Build + zip dist/ into $(PLUGIN_ID)-$(VERSION).zip for distribution
	@rm -f $(PLUGIN_ID)-$(VERSION).zip
	cd dist && zip -r ../$(PLUGIN_ID)-$(VERSION).zip . >/dev/null
	@echo "✅ Packaged $(PLUGIN_ID)-$(VERSION).zip"

docker: build ## Build Grafana image with the plugin preinstalled
	docker build -t tradik/mddb-grafana-datasource:$(VERSION) .

clean: ## Remove build artefacts and dependencies
	rm -rf node_modules dist build coverage *.zip
