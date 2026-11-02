# Issue 52 — workspace workflows

Plan: (1) versioned workspace stages and migration, (2) shared transition rules
and durable execution commands, (3) API and dynamic board/settings, (4) verification.

Original implementation: one ordered workflow per workspace; stable stage IDs;
`status` is the task's stage ID for API compatibility. Multiple workflows now
extend this design: tasks select a workflow, and integrations select which one
their imports use. Stage IDs are scoped to workflow. Stages run left to right:
tasks start at the first stage and source-closed tasks go to the final stage.
Stage entry snapshots its workflow. Agent success only advances when enabled;
manual completion follows the same rule. Failures never advance or retry
automatically. Final-stage tasks cannot reopen. Configuration edits apply to
future entries. Occupied stages cannot be removed.

Progress: implemented. Multiple workspace workflows, task workflow association,
workflow-scoped stages, per-integration selection, configuration UI, dynamic
boards and durable transition delivery are complete.

Verification:
- `make check` passed (lint, backend/frontend tests, generated contract check,
  frontend types, Go and Vite builds).
- PostgreSQL integration tests were not available in this verification run because
  no test database or Docker daemon was configured.
- `git diff --check` passed.

Limitations: live multi-stage model-provider execution and a browser journey were
not exercised. The worker currently dispatches sequentially per platform process;
team-member slot allocation remains the separately documented unfinished feature.
Vite reports the existing large-bundle and dependency directive warnings.
