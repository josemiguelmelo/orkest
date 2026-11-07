# User questions

Structured questions use a shared worker lifecycle for task and direct conversations.
They are separate from execution permissions: neither `ask` nor `autonomous`
supplies a question answer. Pending questions retain execution ownership and ordinary
chat messages remain separate queued prompts.

## Ownership and contracts

- `internal/agentctl/interaction` defines question/answer values and durable events.
- `internal/agentctl/run` owns admission, validation, journal/reducer state, waiting,
  answer idempotency, and conservative recovery. Callback channels are transient.
- The runtime observer exposes a question callback. Host callbacks bind it to the
  active prompt context and reject forms containing known protected runtime values.
- Worker wire DTOs, server models, and their adapter mappings remain independent.
- Task/direct-conversation HTTP handlers authorize human answers before forwarding.
  The platform records the answering user; the worker binds the response to the
  original task/conversation, logical session, native session, run, command, request,
  and submission IDs.
- The existing ordered event JSON carries questions and answers; no PostgreSQL
  schema migration is needed. The frontend reconstructs forms/history from events.

`question_requested` is committed before publication. `question_answered` is
committed before notifying the original invocation. An identical submission is
acknowledged without a second answer; conflicting submissions and stale answers
are rejected. A saved answer proves recording, not runtime consumption. Unknown
delivery is preserved rather than resent to a replacement invocation.

An unanswered callback ending produces `question_cancelled`, or
`question_delivery_uncertain` when worker ownership/context is interrupted.
Question waits expose `waiting_for_input`; concurrent permissions take precedence
in the summary state while individual questions remain visible.

## Native ACP forms

The ACP adapter advertises `elicitation.form` only when its host implements the
question callback. It handles `elicitation/create` for the active native session.
The pinned Go SDK drops scope fields from its elicitation union, so this one method
is decoded at the adapter boundary using the SDK's lower-level connection. The SDK
still owns framing, request cancellation, and ordered notification processing.

OpenCode, Codex ACP, and Claude Code ACP use this same handler. Native schema
property keys, option values, custom-note fields, and metadata remain unchanged in
the response mapping. URL elicitation and unbound child-session questions are not
advertised/supported. A child question needs verified parent-command ownership before
it can be accepted; arbitrary session IDs are rejected.

`internal/platform/form` supports a bounded object-form JSON Schema subset: primitive
text/boolean/integer/number fields, string choices (`enum` or labeled `oneOf`), and
arrays of strings. Supported field constraints include length, numeric bounds,
item counts, uniqueness, and standard date/time/email/URI formats. Required fields
are validated on the worker. Additional answer keys, nested objects, external
references, unsupported constraints, and explicitly secret forms are rejected.
Schema, message, and answer payloads are bounded to 64 KiB each; forms have at most
32 fields and choices at most 100 entries. There are at most 4096 recorded questions
per run. Client validation is a convenience; the worker remains authoritative.

Codex deadlines remain runtime-controlled. If a runtime withdraws a pending request,
Orkest closes the callback; it does not choose an answer when time expires.

## Portable MCP path

New Copilot and Kiro sessions receive an `orkest_questions` HTTP MCP server with an
`ask_user` tool, backed by the same callback and journal. This is the fallback path
until native ACP forwarding is established for those pinned runtime profiles.

The endpoint lives inside agentctl's execution environment and binds only
`127.0.0.1`. It requires a private session bearer token. Each prompt supplies a fresh
`commandKey` in runtime-only context. The tool validates that key before binding a
request to the active observer; expired keys cannot target a later prompt. The
platform and UI do not receive the key or token. Runtime activity redacts them.

Tool input includes `commandKey`, a stable `operationId`, `message`, and object
`schema`. Identical operation retries rejoin the existing wait/result; conflicting
reuse fails. HTTP disconnects do not cancel a worker-owned question. Explicit prompt
cancellation invalidates the command key and wakes waits. The tool returns an
explicit `accept`, `decline`, or `cancel` action, never a pending result disguised
as an answer. At most 100 operations are admitted per command.

The loopback endpoint works within both the Docker execution environment and trusted
Local execution without publishing another port. Real CLI tool-discovery, human-duration
timeouts, and container behavior still require live qualification. A CLI that times
out may retrieve the existing operation using the same ID during that command;
Orkest does not fabricate success when a native tool call fails.

## Persistence, compatibility, and deployment

Sessions using the fallback write native manifest v4 and private
`question-tool.json` state (feature version 1). The saved endpoint, token, and
versioned instruction behavior are preserved on restore. Address conflicts, missing
state, or incompatible feature versions fail visibly while retaining the session.
Workers that do not understand v4 reject it.

Existing v1/v2/v3 sessions retain their previous configuration and do not silently
acquire the fallback server or new instructions. Starting a new session enables the
fallback. Native ACP client form support does not change the pinned runtime/model.

Deploy matching platform/UI and worker builds; mixed question protocol versions are
unsupported. Rebuild execution images for new worker behavior. Existing running
workers keep their current capabilities until deliberately upgraded after their
execution ownership is resolved. Do not terminate active workers merely to enable
the feature. Do not downgrade a v4 fallback session to an older worker, delete its
state, or change Kiro's engine to force compatibility.

Platform restart leaves worker-owned waits alive. Browser reconnect replays them.
Worker/runtime loss is different: a question's persisted record does not recreate
its original RPC or MCP invocation. Recovery pauses such a command even if generic
history coverage suggests continuation. Preserve the question, saved answer,
working files, and session, and report uncertainty. No question invocation is
automatically replayed on the assumption of exactly-once delivery.

## Verification

Deterministic tests cover schema/answer validation, ACP scope and form mapping,
concurrent duplicate answers, persistence failures, cancellation, restart guards,
HTTP authorization, MCP authentication/deduplication, and browser controls/history.
The common runtime suite asks a question and consumes `pnpm` before and after
session restore. Copilot/Kiro fixtures use actual HTTP MCP transport; the other
fixtures use ACP elicitation. These tests are not live provider qualification.

For live `make qualify-runtime QUALIFY_RUNTIME=<runtime>`, also set:

- `ORKEST_TEST_QUESTION_PROMPT`: a prompt that requires one structured string
  question with `pnpm` as an offered/accepted answer, then reports the answer.
- `ORKEST_TEST_QUESTION_EXPECTED`: the expected acknowledgement substring.

The suite supplies the test answer and requires an actual question callback and
acknowledgement before and after restore. All other documented qualification
credentials, binaries, prompts, and MCP prerequisites still apply. Missing live
prerequisites are failures/unverified behavior, never fixture-backed support claims.
