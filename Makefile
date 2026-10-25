.DEFAULT_GOAL := help
.PHONY: help setup dev dev-once dev-agentctl dev-web db-up db-down db-logs auth-up lint-install lint fmt check build test-integration test-recovery

GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := bin/tools/$(GOLANGCI_LINT_VERSION)/golangci-lint

help:
	@echo "setup             Install frontend dependencies"
	@echo "dev               Run backend with Air auto-reload"
	@echo "dev-once          Run backend once without auto-reload"
	@echo "dev-web           Run frontend (separate terminal)"
	@echo "db-up / db-down   Start / stop PostgreSQL (retains data)"
	@echo "db-logs           Follow PostgreSQL logs"
	@echo "auth-up           Start local Keycloak"
	@echo "fmt               Format Go code"
	@echo "lint-install      Install pinned golangci-lint locally (requires curl)"
	@echo "lint              Run Go linters and formatting checks"
	@echo "check             Check lint, tests, frontend types, builds"
	@echo "build             Build both Go binaries and frontend"

setup:
	npm --prefix web ci

dev:
	go tool air -c .air.toml

dev-once:
	go run ./cmd/orkest

dev-agentctl:
	go run ./cmd/agentctl

dev-web:
	npm --prefix web run dev

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

auth-up:
	docker compose up -d keycloak

$(GOLANGCI_LINT):
	mkdir -p bin/tools/$(GOLANGCI_LINT_VERSION)
	curl -sSfL https://golangci-lint.run/install.sh -o bin/tools/$(GOLANGCI_LINT_VERSION)/install.sh
	sh bin/tools/$(GOLANGCI_LINT_VERSION)/install.sh -b bin/tools/$(GOLANGCI_LINT_VERSION) $(GOLANGCI_LINT_VERSION)

lint-install: $(GOLANGCI_LINT)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

fmt: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt ./...

check: lint
	go test ./...
	npm --prefix web run typecheck
	$(MAKE) build

build:
	go build -o bin/orkest ./cmd/orkest
	go build -o bin/agentctl ./cmd/agentctl
	npm --prefix web run build

test-integration:
	go test -tags=integration ./tests/integration -count=1

test-recovery:
	@echo "$@: not implemented; restoration and continuation are not verified."
	@exit 1
