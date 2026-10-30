.DEFAULT_GOAL := help
.PHONY: help setup dev dev-once dev-agentctl dev-web db-up db-down db-logs auth-up lint-install lint fmt sqlc check build test-integration test-postgres test-recovery test-recovery-unit docker-agentctl-base docker-agentctl-opencode docker-agentctl-claude-code docker-agentctl-images test-docker check-runtime-boundaries

GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := bin/tools/$(GOLANGCI_LINT_VERSION)/golangci-lint
SQLC_VERSION := v1.31.1
DOCKER ?= docker
AGENTCTL_RUNTIME ?= opencode
AGENTCTL_ARGS ?=
QUALIFY_RUNTIME ?= .+
OPENCODE_VERSION = $(shell python3 -c 'import json; print(json.load(open("internal/core/runtime/versions.json"))["opencode"].removeprefix("opencode v"))')
CLAUDE_AGENT_ACP_VERSION = $(shell python3 -c 'import json; print(json.load(open("internal/core/runtime/versions.json"))["claude-code"])')
AGENTCTL_BASE_IMAGE ?= orkest/agentctl-base:dev
AGENTCTL_OPENCODE_IMAGE ?= orkest/agentctl-opencode:$(OPENCODE_VERSION)
AGENTCTL_CLAUDE_CODE_IMAGE ?= orkest/agentctl-claude-code:$(CLAUDE_AGENT_ACP_VERSION)

help:
	@echo "setup             Install frontend dependencies"
	@echo "dev               Run backend with Air auto-reload"
	@echo "dev-once          Run backend once without auto-reload"
	@echo "dev-agentctl      Run agentctl (AGENTCTL_RUNTIME=opencode|claude-code, AGENTCTL_ARGS=...)"
	@echo "dev-web           Run frontend (separate terminal)"
	@echo "db-up / db-down   Start / stop PostgreSQL (retains data)"
	@echo "db-logs           Follow PostgreSQL logs"
	@echo "auth-up           Start local Keycloak"
	@echo "fmt               Format Go code"
	@echo "lint-install      Install pinned golangci-lint locally (requires curl)"
	@echo "lint              Run Go linters and formatting checks"
	@echo "sqlc              Regenerate PostgreSQL query code"
	@echo "check             Check lint, tests, frontend types, builds"
	@echo "build             Build both Go binaries and frontend"
	@echo "test-runtime-conformance Run shared provider and recovery contracts with race detection"
	@echo "qualify-runtime   Run strict live provider qualification (requires credentials/test endpoints)"
	@echo "check-runtime-boundaries Enforce runtime profile and communication dependencies"
	@echo "test-recovery-unit Run deterministic session/queue/recovery tests with race detection"
	@echo "test-recovery      Also verify real OpenCode continuation (requires explicit model and credentials)"
	@echo "test-postgres     Test PostgreSQL repositories (requires DATABASE_URL or all POSTGRES_* settings)"
	@echo "docker-agentctl-images Build the agentctl base and separate runtime images"
	@echo "test-docker       Run Docker Engine smoke test (builds images; requires Docker Engine)"

setup:
	npm --prefix web ci

dev:
	go tool air -c .air.toml

dev-once:
	go run ./cmd/orkest

dev-agentctl:
	go run ./cmd/agentctl -runtime $(AGENTCTL_RUNTIME) $(AGENTCTL_ARGS)

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
	$(GOLANGCI_LINT) run ./cmd/... ./internal/...

fmt: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt ./cmd/... ./internal/... ./db/...

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

check: check-runtime-boundaries lint
	go test ./cmd/... ./internal/... ./db/...
	npm --prefix web run check:generated
	npm --prefix web run lint
	npm --prefix web run test
	npm --prefix web run typecheck
	$(MAKE) build

build:
	go build -o bin/orkest ./cmd/orkest
	go build -o bin/agentctl ./cmd/agentctl
	npm --prefix web run build

check-runtime-boundaries:
	python3 tools/runtime/check_runtime_boundaries.py

test-integration:
	go test -tags=integration ./tests/integration -count=1

test-postgres:
	@{ test -n "$$DATABASE_URL" || { test -n "$$POSTGRES_HOST" && test -n "$$POSTGRES_PORT" && test -n "$$POSTGRES_USERNAME" && test -n "$$POSTGRES_PASSWORD" && test -n "$$POSTGRES_DB"; }; } || (echo "Set DATABASE_URL or all POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USERNAME, POSTGRES_PASSWORD, and POSTGRES_DB values; start PostgreSQL with make db-up"; exit 1)
	go test ./internal/postgres -run 'TestPostgresRepositories|TestSessionMigrations' -count=1

docker-agentctl-base:
	$(DOCKER) build -f build/docker/agentctl-base.Dockerfile -t $(AGENTCTL_BASE_IMAGE) .

docker-agentctl-opencode: docker-agentctl-base
	$(DOCKER) build -f build/docker/agentctl-opencode.Dockerfile --build-arg AGENTCTL_BASE_IMAGE=$(AGENTCTL_BASE_IMAGE) -t $(AGENTCTL_OPENCODE_IMAGE) .

docker-agentctl-claude-code: docker-agentctl-base
	$(DOCKER) build -f build/docker/agentctl-claude-code.Dockerfile --build-arg AGENTCTL_BASE_IMAGE=$(AGENTCTL_BASE_IMAGE) -t $(AGENTCTL_CLAUDE_CODE_IMAGE) .

docker-agentctl-images: docker-agentctl-opencode docker-agentctl-claude-code

test-docker: docker-agentctl-images
	AGENTCTL_OPENCODE_IMAGE=$(AGENTCTL_OPENCODE_IMAGE) AGENTCTL_CLAUDE_CODE_IMAGE=$(AGENTCTL_CLAUDE_CODE_IMAGE) go test -tags=docker ./internal/executors/docker -run 'TestDockerAgentctlLaunchSmoke|TestDockerSessionRestore' -count=1

test-recovery-unit:
	go test -race ./internal/agentctl/... ./internal/orchestration -run 'Restore|Recovery|Queue|Session' -count=1

test-recovery: test-recovery-unit
	@{ test -n "$$ORKEST_OPENCODE_BINARY" && test -n "$$ORKEST_TEST_MODEL_PROVIDER" && test -n "$$ORKEST_TEST_MODEL_ID"; } || (echo "Set ORKEST_OPENCODE_BINARY, ORKEST_TEST_MODEL_PROVIDER, ORKEST_TEST_MODEL_ID and grant credentials with ORKEST_TEST_PROVIDER_ENV; real continuation is unverified without them"; exit 1)
	go test -tags=integration ./tests/integration -run 'TestOpenCodeHandshake|TestOpenCodeContinuation' -count=1 -v

.PHONY: test-runtime-conformance qualify-runtime
test-runtime-conformance:
	python3 -m unittest discover -s tools/runtime -p '*_test.py'
	$(MAKE) check-runtime-boundaries
	go test -race ./internal/agentctl/... ./internal/orchestration ./internal/core/runtime -count=1 -timeout=3m

# Intentionally strict: credentials, test prompts, and external MCP evidence are
# prerequisites; this target never substitutes fixture tests or skips a runtime.
qualify-runtime:
	ORKEST_RUNTIME_CONFORMANCE=live go test ./internal/agentctl/runtime -run '^TestRuntimeConformance/$(QUALIFY_RUNTIME)$$' -count=1 -timeout=5m -v
