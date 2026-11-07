# Durable user questions: implementation plan

Status: implementation tracked below; original phases retained as the design and
qualification checklist. Implemented behavior is documented in [user questions](user-questions.md).

## Implementation progress (2026-11-07)

| Area | Progress | Verification boundary |
| --- | --- | --- |
| Shared contracts, worker journal/reducer, waiting and idempotent answers | Implemented | Deterministic scope, concurrency, validation, failure, and replay tests |
| ACP forms for OpenCode/Codex/Claude | Implemented | Schema fixtures plus actual ACP subprocess question/answer/follow-up tests; live runtimes unqualified |
| Authorized task/direct-conversation APIs and UI | Implemented | HTTP authorization and frontend form/history/retry tests |
| Private HTTP MCP fallback for new Copilot/Kiro sessions | Implemented | Authenticated HTTP MCP calls in the common runtime fixtures, including after restore; actual CLI behavior unqualified |
| Session compatibility and uncertain-delivery recovery | Implemented | Legacy/new manifest restore plus follow-up; interrupted question invocations pause even with generic continuation evidence |
| Live runtime/model and Docker qualification | Blocked by missing prerequisites | No explicit qualification binaries/models/provider grants/question prompts in this environment; Claude and Kiro executables unavailable |

Implementation choices: Kiro uses the portable fallback until native forwarding can
be established with its pinned executable. Native child-session forms are rejected
until parent ownership is modeled. New fallback sessions use native manifest v4;
v1/v2/v3 sessions preserve their original tool configuration. No database migration
or runtime version change is introduced. `make check` passed, including 58 frontend
tests, generated-client checks, lint, types, and builds. Focused Go race suites passed;
the final runtime-host guard adjustment also passed lint and the host race suite.
See the [verification record](../status.md#verification-record) for scope and live limits.

## Outcome and scope

Agents can ask structured questions in task chat and direct conversations. Users
can answer, decline, or cancel them. Answers reach the original waiting invocation
without starting a new run or changing the pinned session configuration.

Implement one Orkest question lifecycle with two ingress paths:

- Native runtime questions through shared ACP form elicitation for OpenCode,
  Codex, and Claude Code.
- An Orkest-owned HTTP MCP `ask_user` tool for runtimes requiring a portable path.

Target OpenCode, Claude Code, Codex, GitHub Copilot, and Kiro. Support is qualified
per pinned runtime/adapter version and representative provider/model configuration;
sharing ACP or MCP alone does not establish support. Native capability, fallback
tool access, model tool use, and interrupted continuation are separate claims.

Permissions remain a distinct interaction. Permission policy must never synthesize
a question answer. A question wait retains the current execution reservation and
must not trigger workflow completion or advancement.

## Runtime strategy and evidence

The runtime inventory remains authoritative for pinned versions. These findings
describe adapter capability, not completed Orkest integration or live qualification.

| Runtime / pin | Available evidence | Implementation path |
| --- | --- | --- |
| OpenCode 2.1.4 | Documentation describes `elicitation/create` and form capability gating; reported error matches missing client support | Shared ACP handler; verify against pinned executable |
| Codex ACP 2.3.0 | Installed adapter source gates `handleUserInput` on form support and returns an empty answer map without it | Shared ACP handler; test Codex-specific custom-note fields and request timeouts |
| Claude Code ACP 0.99.0 | Tagged adapter source converts `AskUserQuestion` to forms and disables it without form support | Shared ACP handler; test choice/custom-answer mapping, cancellation, and child-session scope |
| GitHub Copilot CLI 1.0.105 | An open upstream issue reports this exact version omits `ask_user` in ACP even with form capability advertised | Qualify Orkest HTTP MCP fallback; do not promise native questions |
| Kiro CLI 2.37.0 | Local V3 MCP elicitation is documented; forwarding through ACP is not established | Probe pinned ACP behavior first; choose native forms if verified, otherwise qualify MCP fallback |

The ACP implementation should be shared, with versioned capability policy in
runtime profiles only where behavior differs. Avoid duplicating the protocol client
for each runtime. Runtime question support and model/provider tool-use behavior
must be reported separately.

An ACP capability advertised by Orkest says what Orkest can receive. It does not
prove that a particular runtime will emit questions. Maintain separate facts for
client support, adapter support, and end-to-end qualification.

## Existing implementation and constraints

- `internal/agentctl/interaction` defines durable events and permission values.
- `internal/agentctl/run` owns journals, the reducer, pending permissions, queues,
  command admission, and recovery decisions.
- `internal/agentctl/runtime/{protocol,contracts}.go` expose permission callbacks
  but no question callback.
- `internal/adapters/runtime/acp/client.go` initializes ACP without form elicitation.
  The installed ACP SDK includes unstable elicitation types.
- `internal/adapters/runtime/runtimehost` owns runtime processes and compatibility
  manifests. Existing sessions must retain their runtime/model/configuration.
- `internal/adapters/mcp/agent_tools.go` hosts stateless platform HTTP MCP tools.
  Existing credentials include task/run scope, expire after 24 hours, and do not
  provide a complete direct-conversation or command-bound question identity.
- Worker transport is defined in `internal/contracts/agentctl/v1`; server domains
  use separate models. Neither server nor worker domains may import the other.
- Task chat already reconstructs permissions from ordered events in
  `web/src/features/tasks/chat-model.ts` and renders them in `task-chat.tsx`.
- OpenCode's current recovery evidence does not establish safe replay of an
  interrupted tool invocation. Session loading alone is insufficient.

## Phase 1: protocol investigation and compatibility decisions

1. Inspect the exact OpenCode 2.1.4 executable/source and ACP SDK version in the
   repository. Capture representative native question requests and responses.
   Current online documentation is a guide, not proof of pinned behavior.
2. Exercise text, labeled choices, multiple selection, grouped fields, required
   fields, and custom answers. Verify OpenCode's optional `<key>_custom` mapping.
3. Capture Codex and Claude native form fixtures from their pinned adapters. Verify
   Codex's auto-resolution deadlines and Claude's custom-answer and child-session
   behavior. Probe Copilot and Kiro using an isolated ACP client that advertises
   forms, then verify HTTP MCP tool discovery, cancellation, and long waits where
   native forwarding is absent. Keep probe sessions separate from saved user sessions.
4. Record a capability matrix with evidence: native forms, MCP questions, maximum
   supported wait behavior, ordinary follow-up, and interrupted question recovery.
   Use unknown/unsupported/unqualified explicitly.
5. Define a bounded form schema subset. Initially support object forms containing
   text, boolean, numeric, single-choice, and multiple-choice fields as needed by
   the captured fixtures; include only types whose validation and rendering are
   implemented. Reject unsupported forms visibly rather than silently dropping
   fields or constraints. Do not advertise URL elicitation.
6. Decide how each adapter binds native/MCP invocations to the original command.
   Do not use a model-supplied run/session/command ID as authorization.

Exit: documented wire fixtures, supported schema subset, per-runtime transport
strategy, and an explicit compatibility policy. Any required runtime upgrade is
separate from enabling the feature on existing sessions.

## Phase 2: normalized contracts and lifecycle

Add question types under `internal/agentctl/interaction`, with independent server
models and explicit wire mapping:

- A stable Orkest question ID and original runtime invocation identity.
- Organization/workspace, task or direct-conversation identity, logical session,
  native session, run, active command, and optional tool-call identity.
- Source transport and adapter format version.
- Title/message, ordered fields, stable field/option IDs, constraints, and custom
  answer rules. Preserve the native response mapping without exposing SDK types.
- Answer action (`answer`, `decline`, `cancel`) and typed field values.
- Answer submission ID, canonical payload hash, actor, and timestamps.

Use stable native property keys as answer keys and preserve option values separately
from display labels. Keep adapter metadata needed to reverse the translation in
private durable state. Validate against the original bounded schema, including
required fields and numeric/string/array constraints; do not rely on browser validation.

Define cancel-question as cancelling only the current question invocation; cancel-run
continues to use the existing run cancellation operation. Declining communicates an
explicit refusal to supply input. Map these distinctions to the supported native
response actions and document runtimes that collapse them. No deadline may select
an answer or turn an unanswered question into an accepted form.

Add a question callback to the runtime observer/protocol boundary and carry it
through the host. Keep constructors explicit; avoid a generic interaction registry.

Persist lifecycle events, tentatively `question_requested`, `question_answered`,
`question_response_sent`, `question_cancelled`, and `question_delivery_uncertain`.
The event names and exact payloads become versioned transport contracts in this
phase. A successful protocol write proves response emission, not model consumption;
only record stronger acknowledgement when the runtime supplies evidence.

Define reducer transitions:

| Transition | Admission rule |
| --- | --- |
| Create pending question | Active matching command; durable append before notification |
| Record answer | Authorized actor; pending matching question; server-side validation |
| Retry identical submission | Return the recorded outcome without another transition |
| Submit conflicting answer | Reject; retain the first committed answer |
| Deliver answer | Original live invocation or verified resumable invocation only |
| Cancel | Serialize against answer admission and close unresolved waits |
| Lose invocation | Preserve question/answer and mark delivery or recovery unresolved |

Support multiple outstanding questions without overwriting a single pending slot.
Derive `waiting_for_input` from unresolved questions while preserving approval and
queue state. Following chat messages remain queued; they are never inferred to be
answers. Answering a question resumes its invocation, not another queued command.

Exit: precise state machine, transport types, validation rules, duplicate semantics,
and fixture coverage for old records without question fields.

## Phase 3: durable worker implementation

Implement pending-question handling beside `conversation_permissions.go`:

1. Use the existing journal and reducer as the authority; in-memory channels only
   wake live callers. Rebuild durable state from the same reducer during replay.
2. Follow existing lock ordering. Keep runtime I/O and blocking waits outside the
   state lock. Serialize answer/cancel/shutdown races with command ownership.
3. Append requests before notifying subscribers, and answers before acknowledging
   HTTP submission or waking the runtime callback. Journal failure fences delivery.
4. Expose pending questions and answer/delivery status through status and ordered
   activity. Preserve receive-only WebSocket semantics.
5. Separate HTTP disconnection from question cancellation. Closing a browser or
   platform connection must not cancel a native runtime question.
6. Implement bounded input sizes, question/field/option counts, and sanitized
   diagnostics. Treat question contents and answers as sensitive conversation data;
   do not collect credentials through forms or log answer bodies.
7. Keep waiting runs active. Audit execution completion and workflow predicates so
   they cannot treat a question or MCP transport error as successful completion.

Exit: deterministic concurrent-answer, cancellation, append-failure, subscriber
reconnect, queue-ordering, and worker-replay checks pass.

## Phase 4: shared ACP adapter and native runtime integration

1. Implement the SDK elicitation callback and verify the request's session and
   original invocation binding. Restrict unsupported child-session requests until
   their ownership is represented and qualified.
2. Validate and translate form schemas to the normalized question contract.
   Preserve exact property keys, choice values, required fields, and custom-answer
   semantics when mapping answers back.
3. Map answer/decline/cancel to the corresponding ACP response actions. Return
   explicit errors for invalid forms; never return fabricated empty success.
4. Advertise `clientCapabilities.elicitation.form` only when the complete durable
   handler is enabled and compatible with the selected adapter/runtime.
5. Keep the capability gate outside permission policy: autonomous execution can
   still need user information.
6. Qualify model follow-through: ask, answer, observe the agent use the supplied
   answer, finish, then send another user message in the same saved session.
7. Run the same ACP handler with Codex 2.3.0 and Claude Code ACP 0.99.0. Add mapping
   fixtures for their actual schemas instead of assuming OpenCode's `_custom`
   convention applies to every adapter. Preserve Codex custom-note fields and
   any supported adapter deadlines without auto-answering.
8. Detect secret-input metadata before publishing a request. Reject forms that
   collect credentials through ordinary task chat, retaining a sanitized reason.
   Claude/Codex input metadata must not be treated as safe merely because OpenCode
   documents avoiding secret forms.
9. Treat child-session elicitation explicitly: either map it to a verified parent
   command with isolated request identity, or reject it visibly. Never attach an
   arbitrary session ID to the current parent session.

Exit: the original error is eliminated for supported forms on the pinned OpenCode
build; OpenCode, Codex, and Claude native requests use the shared handler, UI,
and journal. Enable each runtime independently after qualification.

## Phase 5: platform APIs and shared chat UI

Extend worker HTTP handlers, shared wire values, runner client mapping, orchestration
services, and platform task/direct-conversation routes. Use dedicated question-answer
operations with submission idempotency; do not overload permission endpoints.

Proposed route additions, finalized alongside OpenAPI changes:

- Worker: `POST /agent/questions/respond`, carrying full interaction identity,
  submission ID, action, and typed answer values.
- Platform task run: append `/questions/{questionID}/respond` beneath the existing
  workspace-scoped task-run route.
- Platform direct conversation: append the same suffix beneath the existing
  workspace-scoped direct-conversation run route.

Successful submissions mean the answer is durably recorded; delivery state is
returned separately. Return stable errors for invalid forms/answers, unauthorized
scope, conflicting submissions, closed questions, and blocked recovery. Repeating
an identical committed submission returns its recorded result even if delivery is
now uncertain. Define and fixture the exact statuses in the API contracts.

Authorization must recheck workspace membership and mutation authority at answer
time, including viewers, revoked access, archived conversations, and stale runs.
Scope every request to its original conversation/session/run/question; a run-scoped
MCP credential cannot answer a question as a human.

Update both OpenAPI documents and regenerate the frontend client from source.
Check whether existing event JSON storage can carry the new values without schema
changes. If a migration is necessary, use the repository database-change skill,
version migrations, and regenerate sqlc; do not add a second authoritative question
store merely for UI convenience.

Build a shared question card used by task and direct-conversation chat:

- Render ordered fields with labels, accessible controls, validation, and submit,
  decline, and cancel actions whose effects are explicit.
- Show submitting, recorded, response-sent, cancelled, and unresolved states using
  language appropriate to the user. Do not claim the agent received an answer merely
  because the platform accepted it.
- Render viewers read-only and disable submissions once the question closes.
- Reconstruct pending and answered history from durable events after refresh.
- Preserve drafts across polling updates and request failures; avoid duplicate
  cards when a request arrives through replay and live activity.
- Show multiple questions independently; distinguish ordinary chat from answers.

Exit: both conversation types support the same lifecycle and authorization rules.

## Phase 6: portable HTTP MCP questions

Use the same worker-owned question service for the built-in `ask_user` tool. Prefer
a private HTTP MCP endpoint hosted by agentctl in the execution environment so a
platform restart does not destroy the waiting tool invocation. Verify reachability
and isolation for both Docker and Local executors; do not expose an unauthenticated
worker route. Reuse existing MCP dependencies and explicit interfaces.

Before implementation, settle these concrete protocol requirements:

1. Credentials bind the runtime session and permitted operation, support direct
   conversations, and remain valid or safely refreshable over long waits. Do not
   silently reuse the platform's task-only 24-hour tool claims.
2. The invocation is bound to a command using adapter-controlled context or a
   worker-issued capability validated against the active command. If a runtime
   cannot provide a trustworthy binding, do not qualify this fallback for it.
3. An operation key and canonical payload identify an identical retry. JSON-RPC
   request IDs alone are not durable deduplication keys. Conflicting key reuse fails.
4. An interrupted HTTP connection preserves durable question state. A retry may
   retrieve/rejoin that same operation only after ownership reconciliation; it must
   not create another question or attach to a later command.
5. Prove the runtime tolerates human-duration tool waits. If it cannot, design and
   qualify an explicit create/status/answer-result flow; returning pending from a
   tool must never masquerade as a user answer or release execution ownership.

Provision the endpoint and tool instructions for new compatible sessions. Select
native or MCP behavior per qualified adapter; do not switch an in-flight native
question to MCP or dynamically alter restored session configuration. If a runtime
has an unsupported built-in question tool, determine and qualify its native way to
route questions through Orkest; prompt instructions alone are not a support guarantee.

Exit: the same questions work through MCP, with honest timeout/recovery behavior,
before marking any additional runtime supported.

Copilot is the first fallback qualification target. Its native ACP question gap
must not be confused with failure of Orkest's form handler. Kiro uses the fallback
only if the pinned ACP probe fails to establish a native path. Changes to either
runtime version or Kiro's execution engine require their own session-compatibility
review; never change an existing session's engine merely to obtain question support.

## Phase 7: restart recovery and version compatibility

Distinguish restart boundaries explicitly:

| Boundary | Expected behavior |
| --- | --- |
| Browser reconnect | Replay same pending question and current recorded outcome |
| Platform restart, worker alive | Worker keeps waiting; platform reconnects by cursor |
| Duplicate answer after uncertain HTTP response | Return the original committed outcome |
| Worker loss while runtime may live | Fence replacement; do not infer termination |
| Runtime restart with pending native callback | Rebind only with runtime evidence; otherwise block visibly |
| Answer committed before response write | Preserve answer; reconcile original invocation before delivery |
| Response written before worker crash | Mark uncertain unless runtime history establishes consumption |
| Session loaded without invocation evidence | Preserve state; loading alone does not authorize replay |

Add backward readers for any changed manifest/checkpoint format. Old sessions retain
their model, permissions, instructions, MCP settings, and runtime version. New
question configuration applies to new sessions unless an explicit safe migration
is demonstrated by restore and successful follow-up coverage.

Document rollout order and reject unsupported mixed worker/platform versions before
starting a question-enabled run. Older clients must not silently ignore pending
questions and report the run complete. Maintain existing permission wire formats.

Exit: recovery cases either continue with evidence or preserve state with a precise
blocker; none silently replace the session or fabricate an answer.

## Phase 8: verification and rollout

Run focused deterministic suites while implementing, then `make fmt`, `make lint`,
architecture boundaries, frontend type checks/build, and `make check`. Use race
detection for worker question concurrency. Run integration targets with their
documented prerequisites; report unavailable infrastructure and credentials.

Required observable coverage:

- ACP schema translation and response mapping, including custom answers and errors.
- Multiple pending questions; answer/answer and answer/cancel races; append failures.
- Cross-workspace and stale-session answers, viewers, and revoked access.
- Worker HTTP, platform HTTP, runner mapping, event replay, and OpenAPI compatibility.
- Task and direct-conversation browser flows, reconnect, duplicate submission, and
  validation failures retaining user input.
- MCP disconnect/retry identity, authentication, long waits, platform downtime, and
  direct-conversation scope.
- Legacy session restore plus a successful follow-up using its pinned configuration.

For every pinned runtime, run a credentialed qualification scenario: discover the
question path; ask text and choice questions; consume answers correctly; decline;
cancel; wait while the platform restarts; complete and send a follow-up. Separately
qualify worker/runtime restart behavior, recording supported continuation or the
expected visible recovery blocker. Include Docker and Local boundary checks.

Publish a per-runtime evidence matrix in `docs/status.md` and runtime setup docs.
Record exact versions, model/provider, executor, date, and credential-dependent
gaps. Deterministic fixtures never establish live provider qualification.

## Suggested implementation slices

1. Protocol captures, compatibility policy, normalized contracts, and state machine.
2. Durable worker question handling, shared ACP handler with OpenCode/Codex/Claude
   fixtures, and worker API.
3. Platform authorization/API, generated client, task/direct-conversation UI.
4. Recovery, old-session continuation, and live qualification of OpenCode, Codex,
   and Claude independently. Enable a runtime only when slices 1–4 pass for it.
5. Private MCP endpoint, secure command binding, tool provisioning, fallback tests,
   and Copilot qualification.
6. Kiro native capability probe and qualification through its verified native or
   MCP path; publish the final matrix and remaining support limits.

### Change map

| Slice | Primary code areas | Reviewable result |
| --- | --- | --- |
| Contracts | `internal/agentctl/interaction`, `internal/agentctl/runtime`, `internal/contracts/agentctl/v1` | Typed questions/answers, capability values, versioned wire fixtures |
| Worker lifecycle | `internal/agentctl/run`, `internal/agentctl/run/model` | Journal/reducer transitions, waiting state, idempotency, cancellation/recovery |
| ACP | `internal/adapters/runtime/acp`, `internal/adapters/runtime/runtimehost`, runtime profiles | Form negotiation, schema mapping, original-invocation response, compatibility gate |
| Transport | `internal/adapters/http/agentctl`, `internal/adapters/runnerclient/agentctlhttp` | Answer route, status/events, explicit worker/wire mappings |
| Platform | `internal/server/orchestration`, `internal/adapters/http/server`, existing persistence adapters as needed | Authorized task/direct-conversation answer use cases and event projections |
| Frontend | `web/src/features/tasks/chat-model.ts`, `task-chat.tsx`, shared question controls, generated API client | Pending forms, answer history, validation, reconnect and viewer behavior |
| MCP | Worker HTTP composition, MCP adapter, runtime provisioning, `cmd/agentctl` wiring | Private authenticated fallback using the same question service |
| Verification/docs | `tests/integration`, runtime conformance suites, `docs/status.md`, runtime reference docs | Evidence per pinned runtime, upgrade restrictions, precise recovery limits |

Each slice includes its required tests and documentation. Keep the feature disabled
until the platform, worker, and UI agree on the contract. Any storage migration or
new externally reachable endpoint must be included explicitly in the corresponding
slice's deployment notes.

Update this document after each slice with completed work, verification, and
blockers. Update requirements and architecture only for accepted decisions; keep
unqualified capabilities and proposed behavior labeled explicitly.

## Acceptance criteria

- Supported runtimes can ask and consume answers through the same Orkest UI.
- Requests/answers survive platform restarts without duplicate questions or runs.
- Waiting never releases execution ownership or advances the workflow.
- Unauthorized, conflicting, stale, and cross-scope responses cannot affect execution.
- Cancellation terminates waits without answering a different invocation.
- Uncertain runtime delivery blocks visibly and preserves working/session state.
- Existing sessions restore and accept a follow-up with their original configuration.
- Documentation distinguishes live-qualified support from fixtures and unavailable checks.

## References and provenance

OpenCode's current ACP documentation describes `elicitation.form`,
`elicitation/create`, and custom field semantics:
https://dev.opencode.ai/v2/docs/cli/acp/#questions
Validate these details against the pinned executable in phase 1.

- Codex: local installed `@agentclientprotocol/codex-acp` package version 2.3.0,
  `dist/index.js`, specifically `handleUserInput`, `requestUserInputElicitation`,
  and `buildUserInputRequest`. Capture these behaviors as fixtures before relying
  on them; an installed bundle is not live integration evidence.
- Claude's tagged adapter source:
  https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.99.0/src/acp-agent.ts
  (`handleAskUserQuestion` and form-capability gating).
- Copilot's upstream question transport issue:
  https://github.com/github/copilot-cli/issues/2109
  An October 29, 2026 user report names 1.0.105. This is reported behavior, not a
  locally reproduced qualification result.
- Kiro local V3 MCP elicitation release notes:
  https://kiro.dev/changelog/cli/
  Local support does not establish ACP forwarding for pinned 2.37.0.
