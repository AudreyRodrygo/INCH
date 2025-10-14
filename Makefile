.PHONY: all build test test-integration lint fmt proto-gen docker-up docker-down clean help

# Default target
all: lint test build

# ─── Build ──────────────────────────────────────────────────────────────────

SENTINEL_SERVICES := event-collector event-processor alert-manager notification-dispatcher sentinel-agent
HERALD_SERVICES := gateway-api delivery-worker
ALL_SERVICES := $(SENTINEL_SERVICES) $(HERALD_SERVICES)

BIN_DIR := bin
LDFLAGS := -ldflags="-s -w"

build: ## Build all services
	@echo "==> Building all services..."
	@mkdir -p $(BIN_DIR)
	@for svc in $(SENTINEL_SERVICES); do \
		echo "  -> sentinel/$$svc"; \
		go build $(LDFLAGS) -o $(BIN_DIR)/$$svc ./sentinel/cmd/$$svc; \
	done
	@for svc in $(HERALD_SERVICES); do \
		echo "  -> herald/$$svc"; \
		go build $(LDFLAGS) -o $(BIN_DIR)/$$svc ./herald/cmd/$$svc; \
	done
	@echo "==> Done."

build-%: ## Build a specific service (e.g., make build-event-collector)
	@mkdir -p $(BIN_DIR)
	@if [ -d "sentinel/cmd/$*" ]; then \
		echo "==> Building sentinel/$*"; \
		go build $(LDFLAGS) -o $(BIN_DIR)/$* ./sentinel/cmd/$*; \
	elif [ -d "herald/cmd/$*" ]; then \
		echo "==> Building herald/$*"; \
		go build $(LDFLAGS) -o $(BIN_DIR)/$* ./herald/cmd/$*; \
	else \
		echo "Unknown service: $*"; exit 1; \
	fi

# ─── Test ───────────────────────────────────────────────────────────────────

test: ## Run unit tests
	@echo "==> Running unit tests..."
	go test ./pkg/... ./sentinel/... ./herald/... -count=1 -race -timeout 60s

test-integration: ## Run integration tests (requires Docker)
	@echo "==> Running integration tests..."
	go test ./pkg/... ./sentinel/... ./herald/... -count=1 -race -timeout 300s -tags integration

test-coverage: ## Run tests with coverage report
	@echo "==> Running tests with coverage..."
	go test ./pkg/... ./sentinel/... ./herald/... -count=1 -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

bench: ## Run benchmarks
	@echo "==> Running benchmarks..."
	go test ./pkg/... -bench=. -benchmem -run=^$$ -count=3

# ─── Lint ───────────────────────────────────────────────────────────────────

lint: ## Run linters
	@echo "==> Running linters..."
	golangci-lint run ./pkg/... ./sentinel/... ./herald/...

fmt: ## Format code
	@echo "==> Formatting code..."
	gofumpt -w .

# ─── Proto ──────────────────────────────────────────────────────────────────

proto-gen: ## Generate Go code from Protobuf definitions
	@echo "==> Generating protobuf code..."
	cd proto && buf generate

proto-lint: ## Lint Protobuf definitions
	@echo "==> Linting protobuf..."
	cd proto && buf lint

# ─── Docker ─────────────────────────────────────────────────────────────────

docker-up: ## Start infrastructure services
	@echo "==> Starting infrastructure..."
	docker-compose -f docker-compose.infra.yml up -d

docker-down: ## Stop infrastructure services
	@echo "==> Stopping infrastructure..."
	docker-compose -f docker-compose.infra.yml down

docker-logs: ## Show infrastructure logs
	docker-compose -f docker-compose.infra.yml logs -f

docker-ps: ## Show running containers
	docker-compose -f docker-compose.infra.yml ps

# ─── Clean ──────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	@echo "==> Cleaning..."
	rm -rf $(BIN_DIR) coverage.out coverage.html
	go clean -cache -testcache

# ─── Help ───────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
