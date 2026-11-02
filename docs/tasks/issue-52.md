# Issue 52 — workspace workflows

Plan: (1) versioned workspace stages and migration, (2) shared transition rules
and durable execution commands, (3) API and dynamic board/settings, (4) verification.

Original implementation: one ordered workflow per workspace; stable stage IDs;
`status` is the task's stage ID for API compatibility. Multiple workflows now
extend this design: tasks select a workflow, and integrations select which one
their imports use. Stage IDs are scoped to workflow. Stages run left to right:
tasks start at the first stage and source-closed tasks go to the final stage.
Transitions use the latest saved workflow configuration, including changes during execution. Agent success only advances when enabled;
manual completion follows the same rule. Failures never advance or retry
automatically. Tasks in the current final stage cannot reopen through a task
transition. Occupied stages cannot be removed.

Progress: implemented. Multiple workspace workflows, task workflow association,
workflow-scoped stages, per-integration selection, configuration UI, dynamic
boards and durable transition delivery are complete.

Verification:
- `make check` passed (lint, backend/frontend tests, generated contract check,
  frontend types, Go and Vite builds).
- `make test-postgres` passed against an isolated local PostgreSQL database,
  including workflow configuration changes, concurrent edits and migration rollback.
- Race detection passed for workflow, task, orchestration and PostgreSQL packages.
- `git diff --check` passed.

Limitations: live multi-stage model-provider execution and a browser journey were
not exercised. The worker currently dispatches sequentially per platform process;
team-member slot allocation remains the separately documented unfinished feature.
Vite reports the existing large-bundle and dependency directive warnings.

PR review simplification: relational workflow/stage configuration replaces JSONB
and duplicated reference catalogs (consolidated migration 0035). Dedicated transition inputs
preserve concurrent task content edits. Latest configuration governs completion,
with durable command IDs, stage-entry/run fencing and runtime recovery retained.
Branch migrations 0035–0039 are consolidated into a single final relational
0035, with a local reset script for previously applied versions. Added coverage for configuration edits
during execution, concurrent content edits and workflow migration up/down/up.
Verification: `make check`, `make test-postgres`, race detection, and
`git diff --check` passed. Live provider execution and browser journeys remain
unverified.

Migration consolidation verification: `make check` and `make test-postgres`
passed. Fresh schema, rollback/reapply and local version reset are covered by
PostgreSQL tests. The reset script was also exercised against each superseded
schema version 35–39 (including extra workflows), followed by final 0035 apply.
