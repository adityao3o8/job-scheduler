.PHONY: build test test-unit test-integration test-all lint vet up down seed migrate

GO       := go
GOFLAGS  := -count=1
INT_TAGS := -tags=integration
TIMEOUT  := -timeout=300s

## build: compile all Go packages
build:
	$(GO) build ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## lint: vet + build
lint: vet build

## test-unit: run unit tests (no external deps)
test-unit:
	$(GO) test $(GOFLAGS) -v ./pkg/... ./internal/worker/...

## test-integration: run integration tests requiring Docker (testcontainers)
test-integration:
	$(GO) test $(INT_TAGS) $(GOFLAGS) $(TIMEOUT) -v ./internal/...

## test: unit + integration (the CI default)
test: lint test-unit test-integration

## test-all: Go tests + dashboard build
test-all: test
	cd dashboard && npm ci --silent && npm run build

## up: start all services via docker-compose
up:
	docker compose up --build -d

## down: tear down all services and volumes
down:
	docker compose down -v

## seed: populate demo data (requires running services)
seed:
	./scripts/seed.sh

## migrate: apply migrations against DATABASE_URL
migrate:
	MIGRATIONS_DIR=./migrations $(GO) run ./cmd/migrate

## help: print this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
