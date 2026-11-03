# Artifact publication and input snapshots

Formats, limits, examples, bindings: [task artifacts](../guides/artifacts.md).

## Publication transaction

- Task lock atomically commits publication/revision advancement.
- Task-unique idempotency key: identical actor/run/input returns original revision;
  stale revision or changed input → 409.
- Dispatch mints run-scoped MCP credentials. New publication checks task/workspace,
  nonterminal launch, stage-entry version, current membership. Identical replay remains
  readable after run end; browsers cannot supply producing-run identity.
- Lifetime/signing keys: [storage](../operations/storage-and-secrets.md#protect-credentials).

## Input snapshot transaction

- Append named inputs/expected outputs to all prompts, including explicit/continued work.
- Reservation transaction stores exact prompt in outbox and links selected revision IDs to run.
- Later edits cannot change inputs; recovery reuses delivery without re-resolving revisions.
- Links stay internal; run-history API shapes remain unchanged.

See [workflow implementation](workflows.md), [session persistence](sessions.md),
and [testing](testing.md).
