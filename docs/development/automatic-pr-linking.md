# Automatic PR linking implementation

Status: implemented and verified locally. Branch names remain agent/user controlled.

The executor writes credential-free, relative checkout identity. The Git adapter
inspects repositories and worktrees within the task root. The agentctl observer
compares snapshots, journals changes before publication, and persists task baselines
across runs. The platform PR association service matches exact repository/branch/
commit evidence; provider reconciliation recovers missed events. Existing trigger
and workflow services retain authorization, transition and execution ownership.

Worker observation configuration lives in the durable checkout manifest instead
of runtime configuration, preserving existing session fingerprints. Existing agent
activity storage supplies branch evidence; no separate branch event broker or
projection is required. PostgreSQL transactions serialize association conflicts and
bind pending deliveries without doing provider I/O under locks. Accepted PR
ownership is stable across integrations within a workspace. Unlink suppression
prevents automatic recreation.

Implementation includes migration 0042, sqlc regeneration, optional activity DTOs,
OpenAPI/frontend regeneration, delivery/discovery diagnostics, and a shared complete
transition conversion (fixing the omitted Start flag in same-stage trigger results).
Repository integration fixtures now use isolated schemas; historical migration
rollback coverage includes the new migration dependencies.

Verification:
- `make check` passed: architecture boundaries, lint (zero issues), Go tests,
  generated frontend contract validation, frontend lint/tests/typecheck and builds.
- `make test-postgres` passed using local credentials and temporary schemas.
- Race tests passed for agentctl run/observer, Git adapter, PR matching, GitHub
  discovery, and real PostgreSQL repository concurrency.
- Fixtures cover packed refs, branch/tag name collisions, worktrees, task-root
  containment, baseline/failed-write replay, restored session follow-up, provider
  recovery identities and configuration activation, delayed observations,
  concurrent association, repository selection, conflicting tasks, suppression,
  stale-stage fencing and migration rollback.
- `git diff --check` passed.

Operational limits: automatic matching currently supports same-repository GitHub
Cloud PRs. Unknown/fork heads, pre-existing baseline branches, and branches created
and deleted between scans remain unmatched. Discovery is bounded and exposes
failure diagnostics; polling has a 24-hour recovery horizon. Live GitHub webhook
and credentialed pinned-runtime continuation qualification was not performed.
Existing local schema drift in migration 0040 was encountered during the initial
repository test attempt; fresh isolated migration-chain tests pass, without
rewriting that historical migration or altering its product data.

See [the integration guide](../guides/task-trigger-integrations.md#automatic-association-and-recovery)
for scan bounds, matching rules, upgrade/rollback behavior, and diagnostics.

## Policy/persistence boundary refinement

Trigger configuration, resource link changes, event ingestion, PR association and
trigger processing now follow explicit read → plan → commit operations. Domain
planners own branch eligibility, ownership conflicts, suppression intent, ingestion
status, resolution expiry/retry, stale-stage handling and workflow decisions.
Repositories return facts, acquire locks in commits, compare the current snapshot
with the supplied one, and persist the supplied plan atomically. Reads do not
claim/consume inbox deliveries or write decisions. Changed snapshots produce no
writes; the next worker pass plans again. No trigger/link repository port accepts
a policy callback, and repositories never invoke domain planners.

Configuration writes explicitly carry the reconciliation-reset intent and pinned
workflow definitions. Ingestion and resource writes return conflicts on stale
facts; processing and resolution return an unapplied result for a superseded plan.
The workflow snapshot loader and persistence writer are shared with existing task
transactions to avoid duplicating locking/persistence logic. Existing unrelated
CRUD transaction APIs retain their prior contracts.

Verification for this refinement: full `make check`, real `make test-postgres`,
and domain/PostgreSQL race tests passed. Added checks cover reusable reads,
changed evidence/delivery state, concurrent commits, deterministic planning,
expiry boundaries, explicit suppression, and timestamp precision tolerance.
No new migration, wire format, or runtime/session configuration change is required.
