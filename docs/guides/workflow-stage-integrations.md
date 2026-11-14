# Workflow stage integrations

A workspace integration connects external provider activity to one task's current
workflow stage. The initial provider is GitHub Cloud pull requests. Task imports
and coding integrations remain separate; a stage integration references an existing
workspace coding connection for API reads.

## Connect GitHub

Workspace Admins and inherited organization administrators can manage connections.
Members and Viewers can inspect authorized connection/status data.

1. Register the GitHub repositories through a workspace coding integration. Its
   token must be able to read their pull requests and reviews.
2. In **Workspace settings → Workflow stage integrations**, create a connection,
   select the coding integration and allowed registered repositories, and supply
   a random webhook secret of at least 32 characters. Secrets are encrypted and
   write-only; omitting the secret on update preserves it.
3. Copy the callback URL. Configure repository webhooks manually in GitHub, using
   that URL, JSON payloads and the same secret. Subscribe to **Pull requests** and
   **Pull request reviews**. The deployment needs reachable HTTPS ingress.
4. Validate and enable the connection. Webhook validation and API credentials are
   separate checks: a valid signature alone grants no task ownership.

There is one provider-neutral endpoint:

```text
POST /callbacks/workflow-stage-integrations/{connectionID}
```

The stored connection selects its adapter. The HTTP handler preserves original
bytes and headers. The adapter authenticates notifications, extracts delivery
identity and normalizes events; the handler contains no GitHub-specific logic.
A supported notification receives `202` only after durable acceptance; a duplicate
or authenticated ignored notification receives `200`.

## Link a task's pull requests

The agent must publish a task artifact with kind `code_review` or the compatible
`pull_request` kind, using an external GitHub PR URL. The existing task-scoped
`publish_artifact` tool records its task/run identity.

Declare a producing stage output and a named input on the consuming stage. Select
that input in the integration rule. The platform verifies each PR through GitHub
and checks its immutable base repository identity against both the task's selected
repositories and the connection's repository allowlist. PR titles, branch names,
descriptions and arbitrary artifact metadata never establish ownership.

The consumer stage pins the verified set of artifact revisions. Late publication
can activate an unresolved observation; subsequent revisions do not silently
replace an already pinned set. Re-entering the stage creates a new observation.
The first successful verification records a baseline and does not replay activity
from before activation. Observation activation is asynchronous, so activity before
that baseline is not an eligible trigger. An empty PR set never satisfies a merge.

## Configure a stage rule

In the workflow editor, expand **External integrations** on the consuming stage
and select **Add integration callback**. Under **When this happens**, choose the
connection, event and matching parameters. Under **Then do this**, choose a
response and its destination stage or agent prompt. The summary updates as you
edit; paused rules remain editable. Save the workflow to apply changes. Existing
events are not replayed.

Only one enabled rule per stage, connection and action is permitted.

| GitHub action | Parameters / behavior |
| --- | --- |
| Review added (`review_added`) | Submitted reviews, filtered by review state. Optional current-head requirement ignores reviews for an older commit. Inline comments, review requests, edits and dismissals do not count as new submitted reviews. |
| PR merged (`pr_merged`) | A closed PR explicitly marked merged. Choose any or all linked PRs; all is the default. Closing without merging never matches. |

Each rule has one callback:

- **Move task:** choose any workflow stage as the destination, including the
  current or closed stage. Existing stage assignment and entry automation apply
  after movement; choosing the current stage leaves the task in place.
- **Call assigned agent:** enter a custom prompt. The task stays in its current
  stage for this callback. Bounded external PR/review context is appended as
  untrusted data. The agent can retrieve feedback with its existing credentials;
  review text is not injected into the configured instructions.

Agent calls can continue a compatible conversation or explicitly start a new one.
The continue-or-create policy creates a session only if none exists. Existing
sessions retain pinned runtime/model configuration. A differently assigned or
unverifiable legacy conversation is preserved and reported as incompatible; use
an explicit new-session choice when deliberately replacing the conversation.
Ordinary manual continuation of legacy sessions remains available.

Calls resolve the task's assignment when admitted, then recheck assignment,
conversation, stage, workflow/connection versions and current authority when
reserving the run. Editing an agent definition does not change a continued
session's model. Configuration changes never themselves send prompts.

## Status, failures and recovery

Inspect observation errors and callback outcomes under the connection's status.
Missing inputs or inaccessible PRs leave an observation unresolved. Busy tasks,
approval/input waits and uncertain active execution block effects; they never
release ownership or answer an interaction. Retry eligible blocked events
explicitly after resolving the conflict. Failed or uncertain agent commands must
be reconciled before any user-started replacement; they never automatically retry.

Transport delivery IDs deduplicate webhook redelivery; semantic event IDs also
converge notifications and API reconciliation. Multiple reviews in one stage stay
separate events. PostgreSQL stores receipts, observations and commands durably;
callback planning commits effects and acknowledgement atomically. Reconciliation
uses provider API reads to recover missed eligible activity with an overlap window.
Provider outages and rate limits remain visible. Transient activity no longer
available from the provider cannot be reconstructed as a complete historical audit.

Saving a workflow or connection invalidates old configuration observations. New
observations establish a fresh baseline, without replaying historical activity.
Removing membership or changing configuration prevents pending callbacks from
acquiring authority under old settings. A GitHub sender is audit context, never an
Orkest authenticated user; callbacks use the configuring administrator's authority.

Automatic callbacks cannot reopen closed tasks or move tasks into the closed
stage. Marking done remains a user operation. Automation continues only the same
task and does not solve the existing unfinished team-member slot allocation.

## Adapter extension

Implement `WorkflowStageIntegrationProvider`, plus subject verification/reconciliation
ports, then register the adapter in `cmd/server`. Providers own configuration and
action schemas, authentication, external identity and matching. GitLab can supply
its own merge-request actions through these contracts without a new webhook route
or provider branches in workflow execution.

See [implementation](../development/workflow-stage-integrations.md) and
[workflow delivery ownership](../development/workflow-delivery.md). Private GitHub
access, real webhook delivery and credentialed model continuation require live
qualification beyond deterministic fixtures and local PostgreSQL tests.

Activity shows the fifty most recent observations and effects. Use **Load older
activity** to inspect earlier history. **Check repository access** verifies access
to every selected repository and its pull requests. Pending review and merge
conditions are reverified at most once per ten seconds; periodic reconciliation
continues to recover missed webhook events.
