# Task trigger integrations

Trigger integrations receive signed external webhooks and run configured actions
for explicitly linked workspace tasks. Actions can move a task between workflow
stages or start its assigned agent while keeping the current stage. Source integrations continue
to own imported task content; coding integrations continue to own repository
credentials. Trigger integrations do not import tasks or change source status.

## Configure GitHub

1. Register the GitHub repository through a coding integration and select it on
   the task. The numeric GitHub repository ID must match the trigger configuration.
2. Open Integrations → Trigger integrations. Choose GitHub, enter the repository
   ID and a random secret of at least 32 characters, and configure movement rules.
3. Copy the webhook URL into GitHub repository webhook settings, choose
   `application/json`, set the same secret, and select Pull requests and Pull
   request reviews. Use HTTPS for the externally reachable platform URL.
4. Under Links and activity, link the task and PR number **before** the event.
   One PR maps to one task per integration. Multiple PRs may map to a task.
5. Configure the destination stage's assignment, custom instructions and entry
   start rule if the event should initiate agent work.

To have a submitted review call the assigned agent without moving the task, add a
rule for `pull_request.review_submitted`, select the current stage, choose **Start
assigned agent in this stage**, and enter a custom prompt. The agent continues its
task session when compatible and can use the authenticated `gh` CLI scoped to the
task's selected GitHub repository to fetch the review and inline comments. It can
then make changes and reply to comments. A same-stage run requires an assigned
agent, an idle task, and administrator access that remains valid when processing.

For example, review submission in In review can move a task to Address review;
its stage prompt can instruct the assigned agent to inspect and answer comments.
Or it can start the agent in In review with a rule-specific prompt. The start
receives a generated PR reference, not an injected webhook comment body. A merge
event in Awaiting merge can move a task to Done without starting work.

GitHub review submission means every published review, including approvals and
comment-only reviews. Draft/edited/dismissed reviews and closed unmerged PRs do not
trigger work. Provider conditions such as review state are not implemented.
For multiple linked PRs, a matching individual PR event applies the rule; there
is no implicit all-PRs-merged gate. GitHub Cloud is the initial adapter.

## Behavior and access

- Workspace administrators and inherited organization administrators configure
  integrations and links. Workspace readers may inspect configuration and activity;
  secrets are encrypted and never returned. Empty update secrets preserve the
  current secret. Configuration updates require the current version.
- Rules select an event, workflow, source stage and different destination stage.
  Only one rule may match an event/workflow/source stage in an integration.
  Referenced stages cannot be deleted while the integration rule exists.
- Matching uses current configuration. Saving configuration never starts work.
  Disabling an integration also prevents its queued deliveries from moving tasks.
- Ingestion verifies HMAC-SHA256 on the original body and limits bodies to 2 MiB.
  The public webhook URL is not a credential. Browser/OIDC sessions do not
  authenticate webhook deliveries.
- Acknowledgement follows durable PostgreSQL ingestion. Repeated delivery IDs and
  semantic event identities do not create repeated effects. Unsupported events
  and GitHub pings are acknowledged without creating work.
- Missing links, events before linking, events older than the current stage entry
  (with one second timestamp precision tolerance), and changed stage versions are
  ignored. Delivery audit keeps up to the latest 100 entries in the API/UI.
- Task links and selected repositories are checked again during processing.
  The configuring administrator's access is also rechecked.
- Active/queued execution or revoked access blocks movement. Blocked deliveries
  require user intervention; they never interrupt agents, resume cancelled work,
  or automatically retry an execution. Start failures remain visible through the
  existing workflow execution state.
- Automated starts use a new session, as existing stage-entry automation does.
  This implementation does not change continued sessions or their pinned models.
- Unlinking prevents pending deliveries from applying while they remain unlinked.
  Relinking never remaps an already accepted delivery to another task.

## API and extension contract

The authenticated API exposes provider discovery and workspace-scoped integration
CRUD, links, and delivery history. See the served OpenAPI contract for schemas.
Webhooks use `POST /webhooks/task-trigger-integrations/{integrationID}`.

Implement `TaskTriggerProvider` in `internal/adapters/integrations/tasktriggerintegration/providers/<provider>`:
provide a descriptor/configuration schema, validation, verified decoding, and
resource validation. Return normalized event identities, occurrence times and
safe external reference URLs. Do not return secrets or raw comment bodies.
Register the adapter in `cmd/server/wiring.go`. Provider IDs and event names are
strings discovered from descriptors; storage, routes, rule evaluation and form
rendering contain no provider switch. The UI currently supports the same string
and string-array configuration schema subset as source integrations.

`internal/server/tasktriggerintegration` owns policy and repository ports. Its
processor calls the existing pure workflow transition planner. The PostgreSQL
adapter locks delivery, integration, workflow and task, then commits stage state,
transition history, stage-entry events and delivery acknowledgement together.
External execution remains outside database transactions, through the existing
fenced workflow command outbox. A crash rolls back the inbox transaction; retry
cannot duplicate a committed transition. PostgreSQL polling needs no broker.

GitHub signature and event conventions follow the
[GitHub webhook documentation](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries).
