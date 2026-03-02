.PHONY: help dev-start dev-stop dev-logs dev-build dev-clean test lint

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev-start: ## Start all services in development mode
	@echo "🚀 Starting MDDB development environment..."
	docker-compose -f docker-compose.dev.yml up -d
	@echo "✅ Services started!"
	@echo ""
	@echo "📍 Available services:"
	@echo "   - MDDB Server:  http://localhost:11023"
	@echo "   - MDDB Panel:   http://localhost:3000"
	@echo "   - MCP Server:   http://localhost:9000"
	@echo "   - gRPC:         localhost:11024"
	@echo ""
	@echo "🔑 Default credentials: admin / admin123"
	@echo ""
	@echo "📊 View logs: make dev-logs"

dev-start-with-ollama: ## Start all services including Ollama
	@echo "🚀 Starting MDDB with Ollama for vector embeddings..."
	docker-compose -f docker-compose.dev.yml --profile with-ollama up -d
	@echo "✅ Services started with Ollama!"

dev-stop: ## Stop all development services
	@echo "🛑 Stopping MDDB development environment..."
	docker-compose -f docker-compose.dev.yml down
	@echo "✅ Services stopped!"

dev-logs: ## Show logs from all services
	docker-compose -f docker-compose.dev.yml logs -f

dev-logs-server: ## Show logs from MDDB server only
	docker-compose -f docker-compose.dev.yml logs -f mddbd

dev-logs-panel: ## Show logs from MDDB panel only
	docker-compose -f docker-compose.dev.yml logs -f mddb-panel

dev-logs-mcp: ## Show logs from MCP server only
	docker-compose -f docker-compose.dev.yml logs -f mddb-mcp

dev-build: ## Rebuild all Docker images
	@echo "🔨 Rebuilding Docker images..."
	docker-compose -f docker-compose.dev.yml build --no-cache
	@echo "✅ Build complete!"

dev-clean: ## Stop services and remove volumes
	@echo "🧹 Cleaning up development environment..."
	docker-compose -f docker-compose.dev.yml down -v
	@echo "✅ Cleanup complete!"

dev-restart: dev-stop dev-start ## Restart all services

dev-shell-server: ## Open shell in MDDB server container
	docker-compose -f docker-compose.dev.yml exec mddbd sh

dev-shell-panel: ## Open shell in MDDB panel container
	docker-compose -f docker-compose.dev.yml exec mddb-panel sh

test: ## Run all tests
	@echo "🧪 Running backend tests..."
	cd services/mddbd && go test -v -timeout 30s ./...
	@echo "✅ Tests passed!"

test-coverage: ## Run tests with coverage
	@echo "🧪 Running tests with coverage..."
	cd services/mddbd && go test -v -coverprofile=coverage.out ./...
	cd services/mddbd && go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated: services/mddbd/coverage.html"

lint: ## Run linter
	@echo "🔍 Running linter..."
	cd services/mddbd && golangci-lint run --timeout 5m
	@echo "✅ Linting passed!"

version: ## Show current version
	@echo "MDDB Version: 2.4.0"

.DEFAULT_GOAL := help
