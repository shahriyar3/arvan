.PHONY: help build run-api run-worker run-relay run-mock-operator run-seed test test-race test-integration lint lint-install check migrate-up migrate-down seed demo load-test load-test-stress tidy swagger

GO ?= go
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $(HOME)/go/bin/golangci-lint)
GOLANGCI_LINT_VERSION ?= latest
GOLANGCI_LINT_MODULE ?= github.com/golangci/golangci-lint/v2/cmd/golangci-lint
MIGRATE ?= $(shell command -v migrate 2>/dev/null || echo $(HOME)/go/bin/migrate)
MIGRATE_DATABASE_URL ?= postgres://sms:sms@localhost:5433/sms_gateway?sslmode=disable
TEST_DATABASE_URL ?= postgres://sms:sms@localhost:5433/sms_gateway_test?sslmode=disable
SWAG ?= $(shell command -v swag 2>/dev/null || echo $(HOME)/go/bin/swag)
K6 ?= k6
LOAD_BASE_URL ?= http://localhost:8080
LOAD_TOKEN ?= demo-token-account-a
LOAD_TARGET_RPS ?= 80
LOAD_STRESS_RPS ?= 1000
LOAD_DURATION ?= 30s

help:
	@echo "Targets: build, run-api, seed, demo, load-test, load-test-stress, test, test-integration, check, migrate-up, migrate-down, swagger"

build:
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker
	$(GO) build -o bin/outbox-relay ./cmd/outbox-relay
	$(GO) build -o bin/mock-operator ./cmd/mock-operator
	$(GO) build -o bin/seed ./cmd/seed

run-api:
	$(GO) run ./cmd/api

run-worker:
	$(GO) run ./cmd/worker

run-relay:
	$(GO) run ./cmd/outbox-relay

run-mock-operator:
	$(GO) run ./cmd/mock-operator

seed:
	$(GO) run ./cmd/seed

demo:
	./scripts/demo.sh

load-test:
	@echo "Load test at $(LOAD_TARGET_RPS) RPS (default rate limit: 100 req/s per account)"
	$(K6) run scripts/load/k6-send.js \
		-e BASE_URL=$(LOAD_BASE_URL) \
		-e TOKEN=$(LOAD_TOKEN) \
		-e TARGET_RPS=$(LOAD_TARGET_RPS) \
		-e DURATION=$(LOAD_DURATION)

load-test-stress:
	@echo "Stress test at $(LOAD_STRESS_RPS) RPS — raise RATE_LIMIT_LIMIT first (e.g. 2000) or set RATE_LIMIT_ENABLED=false"
	$(K6) run scripts/load/k6-send.js \
		-e BASE_URL=$(LOAD_BASE_URL) \
		-e TOKEN=$(LOAD_TOKEN) \
		-e TARGET_RPS=$(LOAD_STRESS_RPS) \
		-e DURATION=$(LOAD_DURATION)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration-setup:
	@docker compose exec -T postgres-primary psql -U sms -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sms_gateway_test'" | grep -q 1 || \
		docker compose exec -T postgres-primary psql -U sms -d postgres -c "CREATE DATABASE sms_gateway_test"
	$(MIGRATE) -path migrations -database "$(TEST_DATABASE_URL)" up

test-integration: test-integration-setup
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" $(GO) test -race -p 1 -tags=integration ./internal/service/ ./internal/handler/ ./internal/repository/ ./internal/worker/

lint-install:
	$(GO) install $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)

lint:
	@test -x "$(GOLANGCI_LINT)" || $(MAKE) lint-install
	$(GOLANGCI_LINT) run ./...

check: lint test-race test-integration

migrate-up:
	$(MIGRATE) -path migrations -database "$(MIGRATE_DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(MIGRATE_DATABASE_URL)" down 1

tidy:
	$(GO) mod tidy

swagger:
	$(SWAG) init -g cmd/api/main.go --output api/openapi --parseDependency --parseInternal
