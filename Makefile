.PHONY: all build test test-integration lint fmt proto-gen docker-up docker-down clean help

all: lint test build

# ─── Build ──────────────────────────────────────────────────────────────────

SERVICES := event-collector event-processor alert-manager notification-dispatcher sentinel-agent
BIN_DIR := bin
LDFLAGS := -ldflags="-s -w"

build: ## Build all services
	@echo "==> Building INCH services..."
	@mkdir -p $(BIN_DIR)
	@for svc in $(SERVICES); do \
		echo "  -> $$svc"; \
		go build $(LDFLAGS) -o $(BIN_DIR)/$$svc ./inch/cmd/$$svc; \
	done
	@echo "==> Done."

build-%: ## Build a specific service (e.g., make build-event-collector)
	@mkdir -p $(BIN_DIR)
	@echo "==> Building $*"
	@go build $(LDFLAGS) -o $(BIN_DIR)/$* ./inch/cmd/$*

# ─── Test ───────────────────────────────────────────────────────────────────

test: ## Run unit tests
	@echo "==> Running unit tests..."
	go test ./pkg/... ./inch/... -count=1 -race -timeout 60s

test-integration: ## Run integration tests (requires Docker)
	go test ./pkg/... ./inch/... -count=1 -race -timeout 300s -tags integration

test-coverage: ## Run tests with coverage
	go test ./pkg/... ./inch/... -count=1 -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html

bench: ## Run benchmarks
	go test ./pkg/... -bench=. -benchmem -run=^$$ -count=3

# ─── Lint ───────────────────────────────────────────────────────────────────

lint: ## Run linters
	@echo "==> Running linters..."
	golangci-lint run ./pkg/... ./inch/...

fmt: ## Format code
	gofumpt -w .

# ─── Proto ──────────────────────────────────────────────────────────────────

proto-gen: ## Generate Go code from Protobuf
	cd proto && buf generate

# ─── Docker ─────────────────────────────────────────────────────────────────

docker-up: ## Start infrastructure
	docker-compose -f docker-compose.infra.yml up -d

docker-down: ## Stop infrastructure
	docker-compose -f docker-compose.infra.yml down

docker-logs: ## Show infrastructure logs
	docker-compose -f docker-compose.infra.yml logs -f

# ─── Clean ──────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
