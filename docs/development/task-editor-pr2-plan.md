# Task editor PR 2: existing checkout discovery

Status: implementation in progress; discovery and filesystem qualifications are
in place, real-model continuation remains outstanding.
Last updated: 2026-11-14.

Prerequisite: PR 1, merged as `2b85451`, with its recorded Docker/browser evidence.
This expands [PR 2 in the delivery breakdown](task-editor-first-delivery.md).
The main [task editor plan](task-editor-plan.md) owns confirmed product behavior.

## Outcome and boundary

Given an authorized user and organization/workspace/task identity, resolve only
the task's currently selected, existing repository checkouts and produce a
temporary multi-root workspace manifest suitable for the later Docker backend.
Discovery must leave task files, branches, permissions and session state untouched.

This PR adds no routes, UI, editor containers, mode transitions, database storage,
runtime configuration changes or production enablement. Local and Kubernetes
storage remain outside this increment. PR 3 consumes the discovery result; PR 4
adds lifecycle coordination and rechecks; PR 5 enforces execution handoff.

## Findings from the current code

- Docker `executor.go:taskDirectories` calculates the SHA-256 scope from the JSON
  array `[organizationID, workspaceID, taskID]`. It also creates/chmods work and
  state directories and creates a Codex auth placeholder. It cannot be called
  from editor discovery.
- `checkout.Preparer` builds `repositories/<provider>/<repositoryID>`, creates
  parents and invokes checkout providers, sometimes resolving credentials.
  Discovery must share only pure relative-path validation/calculation with it.
- `TaskService.Get` and the task PostgreSQL repository already return selected
  registrations through workspace-scoped joins. Reuse these rather than scanning
  retained directories or adding a new SQL query.
- `TaskService.AuthorizeExecution(..., true)` already denies Viewers and verifies
  task scope. Use this boundary before accessing task files, then obtain current
  selections. Attachment ownership and lifecycle authorization remain PR 4 work.
- `TestDockerSessionRestore` tests real storage replacement but makes no model
  follow-up. `TestOpenCodeContinuation` does make a follow-up, but currently uses
  unrelated temporary work/state directories. Neither alone proves continuation
  through the storage extraction proposed here.
- PR 1's VM required a disposable-fixture permission workaround. Do not copy
  chmod 0777 into discovery or treat host stat checks as daemon access evidence.

## Implementation sequence

### 1. Preserve and extract Docker storage calculation

Add a small pure storage helper accepting explicit scope values and the normalized
data root, independent of `RunConfig`. Return the exact existing work and state
paths without filesystem operations. Keep it in Docker until another actual
consumer requires wider reuse; editor discovery is now that consumer, so the
calculation lives in `internal/platform/taskstorage` and both call sites use it.

Retain `taskDirectories` as the agent preparation wrapper: call the pure helper,
then preserve its current creation, permissions and Codex-auth behavior. Do not
change runtime-home paths, session hashes, mounts or manifest formats.

First add fixed expected-path fixtures calculated from the historical algorithm,
including scope values requiring JSON escaping. Verify changes to org/workspace/
task isolate paths, while run/session/runtime/model changes do not move task files.

### 2. Share repository path calculation only

Add a pure helper for validated repository relative paths. Because the server
resolver must not import an adapter, put the shared calculation in
`internal/platform/repositorypath` and call it from checkout preparation and
Docker inspection. Reject empty, dot, parent and separator-bearing identities;
retain valid existing provider/registration paths. Keep provider lookup,
credentials, mkdir and checkout operations in preparation.

### 3. Resolve selected registrations through a narrow use case

Introduce `internal/server/taskeditor` with a checkout resolver and small
consumer-owned interfaces for task access and existing-checkout inspection.
Names/signatures are illustrative; avoid introducing the later lifecycle service
or its backend contract prematurely. Use editor-specific types rather than
requiring an agent `RunConfig` or credentials.

Order: authorize mutation for the scoped task, read selected task repository refs,
map their registration ID/provider/display name into inspection inputs, inspect,
then return eligible roots plus structured missing-checkout information. Host
paths remain internal and will not be future API response fields.

Use current task references as the allowlist, never enumerate every checkout.
An empty selection and an unstarted task are ordinary explanatory results.
For a mixed selection, return existing roots and list missing ones explicitly.
An invalid or escaping selected path fails discovery rather than being reported
as missing. Preserve context cancellation and contextual filesystem errors.

### 4. Inspect paths and Git metadata without mutation

Implement the inspector in the Docker adapter using the pure storage helper.
Validate containment component by component from the configured storage root;
do not rely on string-prefix comparisons. Establish the configured root's canonical
identity and reject redirected descendants, including repository-root and parent
symlinks, dangling links and non-directory roots.

Require a usable checkout, including Git metadata. For the initial implementation,
accept ordinary in-root `.git` directories. Inspect `.git` files, `commondir` and
object alternates for external dependencies: report unsupported layouts explicitly
unless all required metadata can be safely exposed within the authorized root.
Do not mount another checkout or agent-state directory to make a linked worktree
work. Document this initial support boundary and test it with real Git fixtures.

Do not follow arbitrary file-content symlinks or recursively rewrite the tree;
these do not authorize extra mounts. Avoid shell execution of workspace content.
Keep validation results as a snapshot: PR 3 must revalidate mount sources before
creation, and PR 4 must handle selection changes under coordination. This PR must
not claim to solve concurrent filesystem replacement with one path check.

### 5. Build the temporary multi-root manifest

Generate deterministic JSON from eligible roots, using readable names with stable
provider/registration disambiguation for duplicate names. Folder paths reference
editor-container destinations, never host absolute paths or a parent directory
containing other repositories. Return the explicit source/destination allowlist.

Separate inspection from materialization: build manifest bytes first, then write
them only to a dedicated disposable directory outside task work/session state.
Use restrictive permissions, explicit ownership/cleanup and failure cleanup.
Do not serialize URLs, credentials or agent configuration. PR 3 binds this file
read-only and provides the temporary editor home/cache.

### 6. Verify compatibility and record evidence

Use focused tests for observable boundaries:

| Area | Required evidence |
| --- | --- |
| Storage | Fixed historical paths; task isolation; no inspection mkdir/chmod/auth files |
| Authorization | Viewer and foreign workspace/task denied before filesystem access |
| Selection | Selected roots only; retained deselected checkout never returned |
| Filesystem | Missing roots, invalid IDs, symlink parents/roots, external Git metadata and access errors |
| Non-mutation | Branch, tracked/untracked bytes, permissions and state remain unchanged; provider/credential spies receive no calls |
| Manifest | Valid deterministic multi-root JSON, duplicate names, container destinations, temporary cleanup |
| Continuation | Pre-extraction layout restores, same session/runtime/model, successful follow-up and retained untracked file |

Use real Git fixtures for metadata behavior. Exercise existing scoped PostgreSQL
task reads when checking selection isolation; a schema/query change is not expected.
If new SQL becomes necessary, apply the database-change skill and revise scope.

For continuation, add a focused integration scenario pairing historical Docker
layout with the extracted helper, restore and a successful pinned OpenCode next
turn. Assert native session and effective configuration identity. Keep worker
types out of server packages; integration code may exercise the separate layers.
Also run the existing real Docker storage replacement scenario. Record host-model
continuation and container storage evidence separately; neither implies a model
follow-up through a replaced container unless actually exercised.

Planned checks: focused Go package tests, architecture boundaries, `make fmt`,
`make lint`, Docker storage restore and `make test-recovery` with the documented
explicit model/provider prerequisites. Skipped live tests do not satisfy acceptance.
PR 3 must separately prove daemon-visible repository/Git access under the intended
non-root identity; PR 2 can inspect permissions but must not change them to fit.

Update the main plan's progress table with exact meaningful results, limitations
and next action. Review the diff for accidental path/session changes and unrelated
files. Full lifecycle/browser qualification remains assigned to later PRs.

## Expected review units

1. Historical path fixtures and pure Docker/repository path extraction.
2. Authorized selection resolver, safe existing-checkout inspection and tests.
3. Manifest preparation, restore/follow-up coverage and evidence documentation.

## Progress and evidence

Implemented so far: extracted the historical task storage hash into
`internal/platform/taskstorage` for both Docker preparation and editor discovery;
shared validated repository-relative path calculation in a platform utility used
by checkout preparation and discovery (preserving server/adapter boundaries);
added an authorized selected-checkout resolver with an injected inspector, a
Docker adapter inspector with symlink/path containment checks, ordinary `.git`
directory support, explicit rejection of linked-worktree and alternate-object
metadata, structured missing registrations, deterministic container-only
manifest paths, and private temporary manifest materialization. Focused tests use
real `git init` fixtures and cover storage hash compatibility, selection/missing
roots, authorization ordering, symlink rejection, external alternates,
non-mutation and temporary cleanup.

Still required before PR 2 is complete: a real pinned OpenCode restore followed by
a successful continuation using the extracted storage helper. PostgreSQL-backed
selection isolation passed for `TestPostgresRepositories/bounded_task_listing`
against a disposable database. The broader `make test-postgres` run reached its
tests but failed in unrelated workflow-delivery and migration-rollback cases. The
pinned OpenCode continuation integration fixture now uses the extracted task
storage directories and asserts session, runtime and model identity, but skipped
because the binary/provider/model settings are unavailable. The real Docker
`TestDockerSessionRestore` passed against Colima
with the pinned image's bundled `opencode/beam-free` selection; it verifies
container replacement, restored session identity, retained untracked work and
fresh-session isolation, but sends no model prompt. Running
`make test-recovery-unit` outside the sandbox passed with race detection; the
earlier sandbox run had failed because loopback question-tool servers could not
bind. Docker images built and the Docker launch smoke test passed against Colima.
The focused PR2 tests, lint and architecture checks pass.

These are implementation steps within one PR. Merge readiness requires the
discovery invariants and continuation evidence above; missing credentials must be
reported as an outstanding acceptance limitation.
