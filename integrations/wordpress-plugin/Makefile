.PHONY: help install lint stan test coverage audit ci clean build

VERSION := $(shell grep -E '^[[:space:]]*\*[[:space:]]*Version:' mddb-sync.php | head -n1 | sed -E 's/.*Version:[[:space:]]*//')

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

install: ## Install Composer dependencies
	composer install

lint: ## Run PHPCS (WordPress security ruleset)
	composer lint

stan: ## Run PHPStan (level 5, WordPress-aware)
	composer stan

test: ## Run PHPUnit
	composer test

coverage: ## Run PHPUnit with coverage (requires xdebug)
	XDEBUG_MODE=coverage composer test:coverage

audit: ## Run composer audit (security advisories)
	composer audit --abandoned=report

ci: audit lint stan coverage ## Run the full CI suite locally

clean: ## Remove build artefacts
	rm -rf vendor coverage.xml clover.xml .phpunit.cache build *.zip

build: ## Build the release zip (mirrors the GH Action)
	@test -n "$(VERSION)" || (echo "Could not derive VERSION from plugin header" && exit 1)
	composer install --no-dev --prefer-dist --no-progress --optimize-autoloader
	rm -rf build && mkdir -p build/mddb-sync
	rsync -a --exclude '.git*' --exclude 'tests' --exclude 'phpunit.xml.dist' --exclude 'phpcs.xml' \
		--exclude 'phpstan.neon.dist' --exclude 'composer.lock' --exclude '.phpunit.cache' \
		--exclude 'coverage.xml' --exclude 'Makefile' --exclude 'CHANGELOG.md' --exclude 'README.md' \
		--exclude 'build' ./ build/mddb-sync/
	(cd build && zip -r "mddb-sync-$(VERSION).zip" mddb-sync) > /dev/null
	@echo "Built: build/mddb-sync-$(VERSION).zip"
