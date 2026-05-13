.PHONY: run build test lint migrate-up migrate-down sqlc-gen swagger

# ─── Configuration ────────────────────────────────────────────────────────────
APP_NAME    := kosku-backend
BINARY_DIR  := bin
BINARY      := $(BINARY_DIR)/$(APP_NAME)
MAIN        := ./cmd/api/main.go
MIGRATE_DIR := db/migrations
DB_URL      ?= $(shell grep DATABASE_URL .env 2>/dev/null | cut -d '=' -f2-)

# ─── Development ──────────────────────────────────────────────────────────────

## run: Start the API server with live reload (requires air)
run:
	@echo "Starting $(APP_NAME)..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		go run $(MAIN); \
	fi

## build: Compile the binary to ./bin/kosku-backend
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BINARY_DIR)
	go build -ldflags="-s -w" -o $(BINARY) $(MAIN)
	@echo "Binary written to $(BINARY)"

# ─── Testing & Quality ────────────────────────────────────────────────────────

## test: Run all tests with race detector and coverage
test:
	go test -race -cover ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

# ─── Database Migrations ──────────────────────────────────────────────────────

## migrate-up: Apply all pending migrations
migrate-up:
	@echo "Running migrations up..."
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL)" up

## migrate-down: Roll back the last migration
migrate-down:
	@echo "Rolling back last migration..."
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL)" down 1

# ─── Code Generation ──────────────────────────────────────────────────────────

## sqlc-gen: Generate type-safe Go code from SQL queries
sqlc-gen:
	@echo "Generating sqlc code..."
	sqlc generate

## swagger: Generate Swagger/OpenAPI documentation
swagger:
	@echo "Generating Swagger docs..."
	swag init -g $(MAIN) -o docs/

# ─── Help ─────────────────────────────────────────────────────────────────────

## help: Show this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
