# Orkest development guide

## Context

Orkest orchestrates SDLC agents with isolated environments, human-directed workflows, task chat, approvals, and durable recovery.

Before implementation, read [requirements](docs/requirements.md) (behavior, scope, recovery, open decisions) and [tech stack](docs/tech-stack.md) (proposed baseline, validation milestones). Follow explicit task instructions; otherwise default to the proposed stack. Preserve confirmed/proposed/open distinctions; never silently promote proposals to requirements. Record significant architecture decisions in the relevant product or technical documentation.

Minimal Go and React/Vite scaffolding exists with PostgreSQL-only Compose and Makefile commands. The standalone `agentctl` OpenCode/ACP launch, REST, WebSocket, and event-journaling prototype exists; platform product services and restart recovery/session continuation remain unimplemented. Create packages as needed, without empty layers.

## Execution

- Inspect context → implement within scope → verify → review diff. Deliver a complete, reviewable result; preserve unrelated user changes and avoid unrelated refactors.
- Make routine, reversible choices autonomously; report consequential assumptions. Clarify open product decisions affecting correctness/acceptance, continuing independent work meanwhile.
- Keep a short plan for substantial work; for cross-session tasks, record progress, verification, and blockers in the task document if present.
- Merge/deploy/publish only with authorization. This file does not authorize messages or external resource creation.
- Report missing credentials/infrastructure/permissions and unverified behavior precisely. Mock success is not real integration verification.

## Stack and boundaries

- Go; Chi/`net/http`; PostgreSQL/`pgx`/`sqlc`; manual constructor injection; `log/slog`. React/TypeScript/Vite frontend. Keycloak/OIDC authenticates; Orkest authorizes.
- One root Go module; entry points `cmd/orkest`, `cmd/agentctl`; frontend `web/`. Small `main.go`; wiring/lifecycle in `internal/app`.
- Feature packages under `internal/`: identity, workspace, task, team, orchestration, integrations, executors, agent runtimes.
- `internal/httpapi`: thin handlers, routing, middleware, browser events. Services own business rules; consumers own interfaces (e.g. task owns repository interface).
- `internal/postgres`: persistence adapters; `db/queries`: SQL inputs; `db/migrations`: migrations; `internal/postgres/sqlc`: generated code. Keep generated types out of business interfaces.
- Keep `api/openapi.yaml` synchronized with the browser API and `api/agentctl.openapi.yaml` synchronized with the standalone agentctl API.
- Hide Docker/OpenCode/GitHub behind contracts; separate shared platform–agentctl messages from platform services.
- Avoid service registries, DI containers, speculative abstractions, or extra infrastructure without concrete need.

## Product invariants

- Enforce organization/workspace isolation for data, subscriptions, credentials, and execution. Authentication is insufficient. Viewers cannot mutate, start work, or answer approvals; do not invent unspecified role rules.
- Users alone assign and explicitly start tasks. Assignment/reopening never starts execution; agent completion never starts another task.
- One slot per member, reserved atomically; input/approval waits occupy it. Release only after the step ends and ownership is resolved.
- Success → review; users alone mark done/reopen. Workflow and execution state differ: failed/interrupted runs leave tasks in progress with a visible indicator; ordinary failures do not auto-retry.
- Imported title/description remain source-managed; local workflow stays separate from source status. Synchronize idempotently.
- Tasks may span registered workspace repositories; retain task branches across review iterations.
- Pin OpenCode and use its default permissions. Forward approvals to users; never auto-approve. Bind responses to task/run/session/request.
- No secrets in commits, task content, chat, API responses, or ordinary logs. Environments receive only needed credentials.

## Recovery

- `agentctl` independently owns OpenCode and ACP stdio; platform shutdown must not stop active agents.
- Persist commands, ordered events, acknowledgement cursors, and pending interactions; reconcile before resend/replacement. Missing heartbeats/disconnection neither prove termination nor release slots/authorize duplicates.
- Persist working trees (including untracked/uncommitted files), branches, sessions, compatible configuration, and working paths outside disposable container layers.
- Infrastructure recovery retains the logical run; user-started revisions after review create new runs. Cancellation never auto-resumes.
- Reconcile uncertain external effects before retry; do not assume cross-system exactly-once execution.
- Test restoration AND continuation against pinned OpenCode/ACP. Session reload alone does not prove continuation; report lost/uncertain progress.
- MVP assumes durable storage survives; complete host/disk-loss recovery is excluded.

## Coding and verification

- Use small explicit constructors, contextual errors, context-aware I/O, coordinated goroutine cancellation/shutdown, and structured logs with execution identifiers and secret redaction.
- Pin dependencies/runtime versions; commit lockfiles. Regenerate `sqlc` from SQL, never hand-edit generated code. Version schema migrations; document destructive operational effects.
- Unit tests beside code; cross-component tests in `tests/integration`. Test observable behavior. Prioritize real PostgreSQL slot/concurrency tests, authorization boundaries, integration idempotency, and requirements §7.3 recovery/approvals. Use race detection for concurrency changes.
- First validate the OpenCode/ACP execution/recovery prototype, before the full UI.
- Before completing Go code or lint configuration changes, run `make lint` and fix findings. Use `make fmt` to apply formatting fixes. `make check` includes `make lint`, so a successful full check satisfies this requirement. Report any blocker that prevents lint from running; never claim an unrun check passed.
- Establish documented local/CI commands as tooling arrives:
  - `make lint`: pinned golangci-lint standard linters and gofumpt formatting checks.
  - `make fmt`: apply gofumpt formatting, including blank lines between multiline functions.
  - `make check`: lint, unit tests, frontend type checks, builds.
  - `make test-integration`: integrations with documented prerequisites.
  - `make test-recovery`: restart, replay, approvals, duplicate prevention.
- `make check` exists. Integration and recovery targets currently fail explicitly because their suites are not implemented. Never mark unavailable/skipped checks as passed.

## Done

Requested behavior/acceptance criteria implemented; relevant checks pass or limitations are explicit; contracts, migrations, generated code, and docs agree; diff has no unrelated changes/secrets. Report changes, verification, and material risks/blockers.
