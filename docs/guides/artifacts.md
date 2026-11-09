# Task artifacts

Workspace/task outputs with extensible kinds (`plan`, `pull_request`, `test_report`),
independent of representation:

- Stored text with media type (100,000-byte limit) or HTTP(S) external reference.
- JSON metadata: 16,000 encoded bytes. Validate `application/json` content.
- Binary uploads/blob storage deferred.

## Publishing and history

- Stable artifact identity, kind, optional output name; immutable numbered revisions
  hold title/content, publisher, optional run, stage ID, and stage-entry version.
- Movement retains artifacts; task deletion cascades to revisions/input links.
- Agents publish through task-scoped `publish_artifact`; browser/user API is read-only.
  `list_artifacts` / `get_artifact` read the same task scope.
- Members may publish; viewers read only. Check task/workspace access on every operation.
  Publication requires current membership and run-bound credentials; browser clients
  cannot supply producing-run identity. Every publish requires a task-unique
  `idempotencyKey`. [Checks](../development/artifacts.md) validate run/stage ownership.

| Operation | Input / result |
| --- | --- |
| Create | `expectedRevision: 0`, no `artifactId` |
| Update | Artifact ID and current revision; stale version → 409 |
| Retry | Same key/actor/run/input returns original revision, even after run end; changed input → 409 |
| Read current | `GET .../tasks/{taskID}/artifacts` |
| Read history | `GET .../tasks/{taskID}/artifacts/{artifactID}/revisions` |

Example:

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

PR artifacts use `kind: "pull_request"`, `content.url`, and optional provider,
repository, PR number/head commit metadata. Links are unverified references, never
fetched snapshots. Publishing neither approves/merges PRs nor completes tasks.
Artifact publication itself does not establish PR ownership. PR association and
webhook automation use [task trigger integrations](task-trigger-integrations.md).

## Stage handoffs

| Declaration | Fields |
| --- | --- |
| `artifactOutputs` | `{name, kind}` |
| `artifactInputs` | `{name, sourceStageId, outputName, required}` |

- Names: lowercase identifiers with digits, dots, underscores, hyphens; unique per
  input/output list within a stage. Bindings reference another stage's existing output.
- Declarations neither impose execution order nor create movement rules.
- Dispatch selects current revisions matching source stage/output/kind from the newest
  matching producing stage-entry version, preserving multiple outputs (such as per-repository PRs).
- Missing optional input → empty array. Missing required input → reject consumer start,
  expose automation failure, retain stage movement. Outputs never gate producer completion.
- Every start gets named inputs/expected outputs, including explicit prompts and
  continued sessions. Inputs are pinned; recovery reuses saved prompt/identities.
  Runs without artifact bindings still receive publishing instructions.
- External URLs may change; use immutable provider IDs in metadata for reproducibility.
- UI: Markdown, text, links, revisions; workflow editor: output/input bindings.
  Render as task data; users open external links, never the platform.

See [transactions](../development/artifacts.md), [rollback losses](../operations/migrations.md#artifacts),
and [testing](../development/testing.md).
