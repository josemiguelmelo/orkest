# Agentctl composition, state, and bounded replay

Status: Accepted for the local communication implementation. This does not add
process/container recovery or platform authorization. Transport responsibilities
are subsequently split by [decision 0007](0007-agentctl-rest-and-websocket.md).
Conversation ownership and locking are superseded by
[decision 0008](0008-conversation-service-composition.md).

RunController composes a concrete Journal and Subscriptions alongside conversation state.
Journal owns private JSONL storage, durable append, and bounded replay. Subscriptions
owns bounded live delivery and slow-reader detachment. RunController coordinates append
before publication and business-state changes. These components share RunController.mu;
filesystem sync remains serialized to preserve ordered acceptance and publication.
A small journal-local durableFile contract permits deterministic I/O failure tests.

A separate turnMu serializes prompt acceptance, completion, and cancellation delivery.
When both locks are needed, turnMu precedes mu. Cancellation releases mu before
provider I/O, allowing status, activity, and permission responses to proceed while
preventing a late cancellation from reaching a subsequent prompt. Shutdown cancels
the service context and waits for owned work. Interrupted state is sticky through
prompt completion; failed/interrupted state survives process exit. processAlive
reports provider liveness independently.

OpenCode startup uses named phases for environment construction, version checking,
run reservation, subprocess ownership, and session initialization. Runtime configuration
uses typed structs. ACP types stay inside the runtime adapter.

HTTP bodies and incoming WebSocket messages are capped at 1 MiB, prompt text at
256 KiB, and serialized journal events at 1 MiB. Command deduplication retains
SHA-256 prompt hashes for at most 4096 identities; it rejects new identities at
capacity rather than forgetting earlier acceptance. Existing duplicates remain
idempotent while the conversation is available.

Replay retains at most 4096 events and 8 MiB of serialized JSON. Old records remain
in the durable journal. Expired/future cursors fail explicitly; clients cannot
silently skip activity. This changes the earlier unlimited replay behavior. A disk
history API and paginated replay are deferred; long disconnects can require manual
inspection of durable history. Disk retention and restart reconciliation remain
unimplemented. Incoming command and outgoing response types are separate, with
stable errorCode values so client behavior need not depend on error prose.
