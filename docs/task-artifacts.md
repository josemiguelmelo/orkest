# Task artifacts

Task artifacts are implemented as workspace- and task-scoped outputs. Kind is an
extensible semantic name (`plan`, `pull_request`, `test_report`, etc.), independent
of representation. A revision contains either stored text with a media type or
an HTTP(S) external reference, plus bounded JSON metadata. JSON content is
validated when its media type is `application/json`. Stored text is limited to
100,000 bytes; metadata to 16,000 encoded bytes. Binary uploads and blob storage
are deferred.

## Publishing and history

Artifacts have a stable identity, kind and optional output name. Title and content
live in immutable numbered revisions. Revisions record the publishing user,
optional producing run, stage ID and stage-entry version. Moving a task never
removes its artifacts. Deleting the task cascades to artifacts, revisions and
input links according to the existing task deletion policy.

The user-facing API is read only. Agents publish through the run-scoped MCP tool. Agent publication creates an artifact using
`expectedRevision: 0` without an `artifactId`; updating one supplies its ID and
current revision number. Concurrent updates reject stale versions with 409.
Every request requires an `idempotencyKey`, unique within the task. Identical
retries by the same actor and producing run return the original revision;
reusing a key with different input returns 409. Publication and revision
advancement commit atomically under the task lock.

`GET .../tasks/{taskID}/artifacts` returns current revisions;
`GET .../tasks/{taskID}/artifacts/{artifactID}/revisions` returns history.
Members may publish; viewers may only read. Task and workspace access is checked
for every operation. Browser clients cannot supply producing-run identity.

Built-in MCP tools `publish_artifact`, `list_artifacts` and `get_artifact` operate
only on the authenticated task. Publishing is exposed only to credentials bound
to a run, minted by dispatch. The platform checks the run's task/workspace,
nonterminal launch state and stage-entry version for new publications. Identical
replays remain readable after the run ends. Existing membership checks still
apply. The original 24-hour tool credential expiry remains unchanged.

Example publish input:

```json
{
  "expectedRevision": 0,
  "idempotencyKey": "publish-plan-1",
  "kind": "plan",
  "title": "Implementation plan",
  "outputName": "implementation_plan",
  "content": {"mediaType": "text/markdown", "text": "# Plan\n..."}
}
```

A PR uses `kind: "pull_request"` and a `content.url`. Metadata may identify the
provider, repository, PR number and head commit. External links are references,
not fetched snapshots or verified provider state. Publishing one neither approves
nor merges it nor marks the task done. Provider validation, PR webhook ingestion
and external-state automation remain deferred.

## Stage handoffs

Workflow stages may declare `artifactOutputs` as `{name, kind}` pairs and
`artifactInputs` as `{name, sourceStageId, outputName, required}` bindings. Names
are lowercase identifiers permitting digits, dots, underscores and hyphens.
Bindings must reference an existing output in another stage. Output names are
unique within a stage; input names are unique within a stage. Configuration does
not impose an execution order or add stage movement rules.

At dispatch, each binding selects current artifact revisions matching the source
stage, output name and declared kind. It selects the newest producing stage-entry
version with matching artifacts, preserving multiple outputs (for example, PRs
for multiple repositories). Optional missing inputs become empty arrays. Missing
required inputs reject the start; existing automation delivery reports the failure
without undoing stage movement. Required means required to start the consumer;
output declarations do not gate completion of the producer.

Dispatch appends named input contents/references and expected outputs to the
prompt, including explicitly supplied prompts and continued-session work steps.
The exact prompt is persisted in the existing prompt outbox, and revision IDs are
linked to the run in the same reservation transaction. Later edits do not change
those inputs. Runs started without bindings still receive publishing instructions.
Recovery reuses persisted prompt delivery rather than resolving newer revisions.
Existing run-history APIs are unchanged; the input links are persisted internally.
External URLs can change independently; record immutable provider identifiers in
metadata when reproducibility matters.

The task UI supports publishing, revising, reading Markdown/text/links and viewing
history. The workflow editor supports output declarations and input bindings.
Content is rendered as task data; external references are opened by the user and
are never fetched by the platform.

## Migration and verification

Migration 0037 adds artifact tables, run input links and stage binding JSON columns.
It preserves existing tasks, runs and workflow behavior. Rollback permanently
removes artifact content, revision history, input links and binding configuration;
export any needed outputs before rolling back.

Validation: `make check`, `make test-postgres` using local connection settings,
and race-enabled Go tests for artifact, MCP, orchestration, workflow and PostgreSQL
packages. PostgreSQL coverage includes concurrent revisions, replay conflicts,
scope isolation, producing-run checks and immutable input references. MCP coverage
exercises publishing over the real HTTP MCP transport. Credentialed live agent
execution and external PR provider behavior are not qualified by these tests.
