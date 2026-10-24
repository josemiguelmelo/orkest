.DEFAULT_GOAL := help
.PHONY: help setup dev dev-web db-up db-down db-logs fmt check build test-integration test-recovery

help:
	@echo "setup             Install frontend dependencies"
	@echo "dev / dev-web     Run backend / frontend (separate terminals)"
	@echo "db-up / db-down   Start / stop PostgreSQL (retains data)"
	@echo "db-logs           Follow PostgreSQL logs"
	@echo "fmt               Format Go code"
	@echo "check             Check formatting, vet, tests, frontend types, builds"
	@echo "build             Build both Go binaries and frontend"

setup:
	npm --prefix web ci

dev:
	go run ./cmd/orkest

dev-web:
	npm --prefix web run dev

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

fmt:
	gofmt -w cmd internal

check:
	@test -z "$$(gofmt -l cmd internal)" || (echo "Run make fmt"; exit 1)
	go vet ./...
	go test ./...
	npm --prefix web run typecheck
	$(MAKE) build

build:
	go build -o bin/orkest ./cmd/orkest
	go build -o bin/agentctl ./cmd/agentctl
	npm --prefix web run build

test-integration test-recovery:
	@echo "$@: not implemented; no integration or recovery verification exists yet."
	@exit 1
