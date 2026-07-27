.PHONY: help build run-api run-worker run-relay run-seed test test-race test-integration lint check migrate-up migrate-down seed tidy

GO ?= go
MIGRATE ?= $(shell command -v migrate 2>/dev/null || echo $(HOME)/go/bin/migrate)
MIGRATE_DATABASE_URL ?= postgres://sms:sms@localhost:5433/sms_gateway?sslmode=disable
TEST_DATABASE_URL ?= postgres://sms:sms@localhost:5433/sms_gateway_test?sslmode=disable

help:
	@echo "Targets: build, run-api, seed, test, test-integration, check, migrate-up, migrate-down"

build:
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker
	$(GO) build -o bin/outbox-relay ./cmd/outbox-relay
	$(GO) build -o bin/seed ./cmd/seed

run-api:
	$(GO) run ./cmd/api

run-worker:
	$(GO) run ./cmd/worker

run-relay:
	$(GO) run ./cmd/outbox-relay

seed:
	$(GO) run ./cmd/seed

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration-setup:
	@docker compose exec -T postgres psql -U sms -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sms_gateway_test'" | grep -q 1 || \
		docker compose exec -T postgres psql -U sms -d postgres -c "CREATE DATABASE sms_gateway_test"
	$(MIGRATE) -path migrations -database "$(TEST_DATABASE_URL)" up

test-integration: test-integration-setup
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" $(GO) test -race -p 1 -tags=integration ./internal/service/ ./internal/handler/

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || $(GO) vet ./...

check: lint test-race test-integration

migrate-up:
	$(MIGRATE) -path migrations -database "$(MIGRATE_DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(MIGRATE_DATABASE_URL)" down 1

tidy:
	$(GO) mod tidy
