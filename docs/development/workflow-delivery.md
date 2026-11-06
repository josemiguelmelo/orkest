# Workflow command delivery ownership

A delivery claim grants permission to reconcile or deliver one
stable workflow command. It never grants ownership of task execution or proves
that an existing runtime has terminated.

1. A short transaction selects an eligible pending command with `SKIP LOCKED`,
   writes a fresh claim token, increments its attempt, and sets a thirty-second
   reconciliation deadline. The transaction commits before any external call.
2. The use case checks durable command acceptance before dispatch. First attempts
   may reserve a run. Reservation locks the command and verifies the current
   claim token before locking the task and atomically writing session/run identity
   and the prompt outbox. A revoked claimant cannot reserve or launch a new run.
3. Executor launch and HTTP/WebSocket communication occur outside database
   transactions. They consume the reserved run and stable command identity.
4. A short acknowledgement transaction locks and verifies the claim token, reads
   execution health under task/run locks, evaluates domain effects, and atomically
   records command state and the conditional stage-work update. Stale claimants
   cannot acknowledge or mutate the successor's state.

After an expired claim is reassigned, the successor reconciles acceptance only.
It never automatically starts that command again. A recorded reservation belongs
to existing launch/prompt recovery with its original identities; disconnection,
claim expiry and acknowledgement failure cannot authorize replacement. If no
acceptance exists, the superseded attempt cannot reserve after token revocation;
record a visible failed work state and `delivery_uncertain` diagnostic requiring
an explicit user action. This deliberately sacrifices automatic retry when a
crash occurred before reservation, rather than retrying an ordinary failed or
uncertain launch. An unavailable acceptance store leaves the claim pending.

Cancellation before acknowledgement leaves the claim for reconciliation. A crash
after reservation, during agentctl acceptance, or after acceptance before the
acknowledgement always reconciles the stable reservation/outbox, without another
launch. Worker lease expiry never releases any existing run/process ownership.

## Upgrade and rollback

Migration 0038 adds nullable/defaulted claim metadata; it rewrites no command JSON,
run/session identities or worker formats. Stop all old platform workflow workers
before upgrading: mixed old/new delivery implementations are unsupported because
old binaries do not honor claim fencing. Independent agentctl workers continue.
Restart the new platform, which reconciles persisted launches and expired claims.
Do not downgrade while claims or uncertain runs are outstanding. The down
migration refuses to drop claim metadata while pending claimed commands exist;
reconcile them before rollback. It does not cancel or terminate agentctl.
