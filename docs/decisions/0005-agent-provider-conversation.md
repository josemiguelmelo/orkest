# Provider conversation and reconnect replay

Status: Accepted for the local communication increment. Process/container recovery
and platform authorization remain unimplemented.

A provider process may implement agentctl's Conversation capability. It accepts
plain text prompts, emits typed text/tool updates, requests typed permission
choices, returns a stop reason, and supports cancellation. ACP types and JSON-RPC
identities remain inside the OpenCode adapter. Filesystem and terminal client
capabilities are not advertised; the runtime owns its tools. Internal reasoning,
raw tool input/output, and runtime diagnostics are not forwarded.

The WebSocket carries prompt, cancel and permission_response commands alongside
existing ping/info commands. Each prompt has a required stable requestId. Same
identity and text is idempotent for the lifetime of agentctl; changed text is
rejected. Only one prompt may be active, including permission waits. Completion
does not start another prompt or alter task workflow. Provider failure or journal
failure interrupts the conversation; no automatic retry or replacement occurs.
Cancellation delivery is serialized against turn completion and subsequent prompts.

Private JSONL files under state-dir/conversations, named by a hash of the full
organization/workspace/task/run scope, record acceptance, events, permission
requests, response delivery intent and results. Records have ordered sequence
numbers and UTC timestamps, and are synced before delivery. Existing files are
refused rather than overwritten. Reconnecting clients pass their last processed
sequence as afterSequence. Replay and subscription are atomic. Event payloads are
sensitive task activity and must not be treated as ordinary logs. No configuration
or credential environment is included in event records.

Connections do not own execution. Bounded subscriber queues detach slow readers;
agent work and journaling continue. Pending permissions wait for a valid explicit
response bound to task/run/session/request and an offered option. Responding twice,
using a stale identity, or selecting an unoffered option is rejected. No automatic
approval occurs. A response record is delivery intent, not proof the runtime has
accepted it. Cancellation or shutdown can dismiss a request without approval.

History and command lookup are retained in memory for replay during this process's
lifetime, in addition to the private journal on disk. Long-running histories have
no retention/compaction limit yet. Client cursors are client-owned, not persisted
acknowledgements. Restarted agentctl does not load journals, restore sessions,
reconcile permissions, or continue execution. The retained launch/history markers
refuse replacement until reconciliation is implemented. This does not satisfy
requirements section 7.3 process/container recovery acceptance.
