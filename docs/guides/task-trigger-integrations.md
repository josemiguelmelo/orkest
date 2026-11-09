# Task trigger integrations

Trigger integrations receive signed external webhooks and run configured actions
for automatically associated workspace tasks. Task associations are automatic. Actions can move a task between workflow
stages or start its assigned agent while keeping the current stage. Source integrations continue
to own imported task content; coding integrations continue to own repository
credentials. Trigger integrations do not import tasks or change source status.

## Configure GitHub

1. Register the GitHub repositories through coding integrations and select the
   relevant repositories on each task.
2. Open Integrations → Trigger integrations. Choose GitHub, enter a random secret
   of at least 32 characters, and configure event rules. No repository list is needed.
3. Configure the integration webhook URL in GitHub at the organization level or on
   individual repositories. Choose `application/json`, set the same secret, and select
   Pull requests and Pull request reviews. Use HTTPS for the externally reachable
   platform URL. A shared webhook may send events for repositories outside this workspace;
   Orkest acknowledges and ignores those events.
4. Run the task normally. agentctl observes new branches in task checkouts,
   regardless of names chosen by user skills. Matching PRs link automatically;
   no per-PR registration step is required. One PR maps to one task within a
   workspace. Multiple PRs may map to a task.
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

On startup, migration 0045 repairs early databases that recorded migration 0041
before rule actions and prompts were added. Existing rules keep their move action
and receive an empty prompt; existing action/prompt configuration is preserved.
Restart the updated server to apply the repair before retrying a failed integration
save reporting a missing `action` or `prompt_text` column. The repair rollback
retains these columns and their configuration.

- Workspace administrators and inherited organization administrators configure
  integrations and may unlink automatic associations. Workspace readers may inspect configuration and activity;
  secrets are encrypted and never returned. Empty update secrets preserve the
  current secret. Configuration updates require the current version.
- Rules select an event, workflow, source stage and different destination stage.
  Only one rule may match an event/workflow/source stage in an integration.
  Referenced stages cannot be deleted while the integration rule exists.
- Repository scope comes from registered workspace repositories, never an integration
  allowlist. A verified event must match the repository's provider identity in the
  integration's workspace and a task association before any rule can apply. Newly
  registered repositories become eligible automatically; no integration edit is needed.
- Matching uses current configuration. Saving configuration never starts work.
  Disabling an integration also prevents its queued deliveries from moving tasks.
- Ingestion verifies HMAC-SHA256 on the original body and limits bodies to 2 MiB.
  The public webhook URL is not a credential. Browser/OIDC sessions do not
  authenticate webhook deliveries.
- The webhook service durably ingests the event and resolves its task association
  before acknowledging it. Resolution uses the event’s repository identity plus
  observed branch/commit evidence within the integration’s workspace. Missing
  evidence remains queued for recovery; conflicting evidence never guesses a task.
- Acknowledgement follows durable PostgreSQL ingestion and association planning. Repeated delivery IDs and
  semantic event identities do not create repeated effects. Unsupported events
  and GitHub pings are acknowledged without creating work.
- Unmatched events wait up to 24 hours for branch evidence, checking every five
  seconds. Conflicting ownership blocks association. Events older than the current
  stage entry (with one second timestamp precision tolerance), and changed stage
  versions, cannot trigger work. Delivery audit keeps up to the latest 100 entries in the API/UI.
- Task links and selected repositories are checked again during processing.
  The configuring administrator's access is also rechecked.
- Active/queued execution or revoked access blocks movement. Blocked deliveries
  require user intervention; they never interrupt agents, resume cancelled work,
  or automatically retry an execution. Start failures remain visible through the
  existing workflow execution state.
- Same-stage starts request compatible session continuation. Stage-entry starts
  retain their existing session policy. Continued sessions keep their pinned models.
- Unlinking suppresses automatic recreation for that integration/PR. The manual
  link creation endpoint is unavailable. Existing accepted deliveries never remap
  to another task.

## API and extension contract

The authenticated API exposes provider discovery and workspace-scoped integration
CRUD, automatic link listing/unlinking, and delivery history. Manual link creation
is not exposed. See the served OpenAPI contract for schemas.
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
service reads a snapshot, calls the pure workflow transition planner, and explicitly
commits the result. Snapshot reads do not consume deliveries. The PostgreSQL
adapter rechecks the snapshot under delivery, integration, workflow and task locks,
then commits stage state,
transition history, stage-entry events and delivery acknowledgement together.
External execution remains outside database transactions, through the existing
fenced workflow command outbox. A crash rolls back the inbox transaction; retry
cannot duplicate a committed transition. PostgreSQL polling needs no broker.

GitHub signature and event conventions follow the
[GitHub webhook documentation](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries).

## Automatic association and recovery

The executor writes a credential-free, relative checkout manifest into the task
working directory. agentctl takes a Git baseline before the prompt, scans every
2 seconds while it runs, and completes a final scan before terminal events. Git
inspection is independent of runtime tools and user skills. It does not create,
rename, push, fetch, or intercept branches, and installs no hooks.

Discovery covers registered task checkouts, linked worktrees, and clones inside
the task root (maximum traversal depth 12 and 100,000 entries; `node_modules` is
excluded). Paths and Git directories outside the root are not followed. Additional
clones must match a manifest repository URL to acquire registered identity;
unregistered checkouts expose a diagnostic without acquiring task permissions.

An association requires a newly observed non-default branch plus exact provider
repository, head branch, and commit evidence from a signed webhook or authenticated
provider response. Baseline branches are not inferred to belong to the task.
Current automatic matching supports same-repository GitHub Cloud PRs; fork heads
remain unmatched. Worktrees share branch evidence with their registered checkout.
A clone/branch created before its first scan remains baseline state. Polling can
miss branches created and deleted between scans; these cases never fabricate
ownership. Unknown/ambiguous activity remains visible in delivery diagnostics.

Snapshots are journaled before publication and cached durably per task across
runs. Restore overlays journal evidence on the cache; incompatible/corrupt state
is reported rather than replaced. Branch disappearance removes matching evidence;
confirmed PR ownership remains stable across deletion and review iterations.

The platform reconciles GitHub PRs and published reviews once per minute using
workspace coding integration credentials. It reads up to five pages of 100 PRs
and five pages of reviews per matching PR, with a 30-second request budget and a
24-hour recovery horizon. Pagination/credential failures expose a discovery
diagnostic. Polling considers events after the current configuration activation;
saving settings does not replay old reviews. Webhook and polling events share
semantic identities and deduplicate through the same inbox. Provider reads may
retry; failed or uncertain agent execution never automatically retries.

This feature is under development and has no existing association data. Configure
GitHub integrations with an empty provider configuration (`{}`). Previously saved
`repositoryIds` are ignored during webhook decoding, polling and processing; editing
an integration removes the retired setting. Earlier observation formats are not
supported. Migration 0043 defines association, suppression and reconciliation
storage. Runtime/model fingerprints are unchanged.
