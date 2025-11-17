.PHONY: help opensearch-up opensearch-down opensearch-logs example-opensearch deps test clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download dependencies
	go mod download
	go mod tidy

opensearch-up: ## Start OpenSearch with Docker Compose
	docker-compose up -d
	@echo "Waiting for OpenSearch to be ready..."
	@timeout 60 sh -c 'until curl -s http://localhost:9200/_cluster/health > /dev/null 2>&1; do sleep 2; done'
	@echo "OpenSearch is ready!"
	@echo "OpenSearch: http://localhost:9200"
	@echo "Dashboards: http://localhost:5601"

opensearch-down: ## Stop OpenSearch
	docker-compose down

opensearch-logs: ## Show OpenSearch logs
	docker-compose logs -f opensearch

opensearch-clean: ## Stop OpenSearch and remove volumes
	docker-compose down -v

example-opensearch: ## Run the OpenSearch example
	@if [ ! -f .env ]; then \
		echo "Creating .env from .env.example..."; \
		cp .env.example .env; \
	fi
	go run examples/opensearch/main.go

example-basic: ## Run the basic example
	go run examples/basic/main.go

test: ## Run tests
	go test -v ./...

test-race: ## Run tests with race detector
	go test -race -v ./...

test-coverage: ## Run tests with coverage
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean: ## Clean build artifacts
	rm -f coverage.out coverage.html
	go clean

format: ## Format code
	go fmt ./...

lint: ## Run linter
	golangci-lint run

build-examples: ## Build all examples
	@for dir in examples/*/; do \
		echo "Building $$dir..."; \
		go build -o /dev/null ./$$dir || exit 1; \
	done
	@echo "All examples built successfully!"

.DEFAULT_GOAL := help
