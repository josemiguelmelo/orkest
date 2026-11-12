# Orkest requirements

Draft product specification; confirmed rules, proposals, and open decisions are labeled.
Section/requirement IDs are stable. See [status](status.md) for implementation and
verification, [glossary](glossary.md) for terms, and [architecture](architecture.md) for ownership.

## 1. Purpose

Orkest orchestrates software-development agents, launches task environments, and
provides monitoring, chat, approvals, and recovery through agentctl and ACP.

## 2. Confirmed MVP scope

- Multiple organizations, users, and workspaces from day one.
- Keycloak authentication: username/password and configured SSO.
- Roles: organization Owner/Admin/Member; workspace Admin/Member/Viewer.
- Extensible adapters for OpenCode, Claude Code, Codex, GitHub Copilot CLI, and Kiro CLI.
- Built-in platform tools; user-defined tools excluded.
- Agent permission policies: `ask` forwards interactive approvals to task UI (default
  for existing agents); `autonomous` uses provider-native noninteractive permissions
  inside the isolated task environment. Native sandbox behavior varies.
- Manual tasks and GitHub Projects v2 issue/draft-issue imports; source-managed
  content remains editable only at the source.
- Explicit assignment/starts, plus configured workflow assignment, stage-entry
  starts, and advancement after success.
- Reusable agent definitions; workspace team members supply execution slots.
- Docker execution by default, behind an extensible executor contract. Local is
  a trusted, single-user host-process option without task isolation. Kubernetes has
  an initial in-cluster executor; see its qualification limits in the
  [executor reference](reference/executors/kubernetes.md).
- agentctl bridges the platform and runtime ACP sessions.
- Task chat, approvals, cancellation, and interruption recovery; see §7.
- HTTP and stdio MCP servers.
- Configurable ordered stages, seeded with todo, in progress, in review, done.
- Multiple registered workspace repositories per task.

See [policy reconciliation](status.md#product-decisions-requiring-reconciliation).
This specification does not override repository instructions.

## 3. Domain model

Rules for the terms in the [glossary](glossary.md).

### 3.1 Organization, User, Workspace

- Keycloak authenticates; Orkest authorizes.
- First login creates a profile keyed by verified subject with username/email;
  later logins do not overwrite it.
- Organization membership is required for workspace membership; invitations are deferred.

| Role | Authority |
| --- | --- |
| Organization Owner | Organization and all memberships; last Owner cannot be removed/demoted |
| Organization Admin | Settings, workspaces, memberships; cannot grant/change/remove Owners |
| Organization Member | Read organization details |
| Workspace Admin | Workspace settings and memberships |
| Workspace Member | Read/edit project resources |
| Workspace Viewer | Read only; no mutations, starts, or approval answers |

- Organization Owners/Admins inherit workspace access.
- Isolate resources, subscriptions, credentials, and executions by workspace,
  including within one organization. No implicit configuration sharing.
- Workspace Admins/inherited administrators manage environment values: ordinary
  values are readable; secrets are encrypted and write-only. Runs retain encrypted
  recovery snapshots. See [environment resolution](guides/agent-configuration.md).

### 3.2 Repository

- Tasks select zero or more registered same-workspace repositories; runs check out only those selected.
- GitHub Cloud, GitLab Cloud, and HTTPS GitLab Server support PAT integration,
  validation, discovery, registration, rotation, and checkout.
- Agents create task branches only in selected repositories needing changes.
  Review iterations retain branches and working trees.
- Imported prompts use the external reference plus the Orkest ID for traceability.
- Agents create/update PRs within permissions. New checkouts use discovered default
  branches; base-branch overrides and further PR policy remain open.
- Commit with application-wide author/committer identity configured through
  `ORKEST_GIT_AUTHOR_NAME` and `ORKEST_GIT_AUTHOR_EMAIL`, defaulting to
  `Orkest Agent <agent@orkest.local>`. Exclude checkout/push credentials
  from repository configuration and plaintext checkpoints. See [credential handling](guides/integrations.md#coding-integrations).
- Task runs receive workspace coding integration credentials for HTTPS Git operations
  even with no linked repositories. Authentication is scoped to the matching provider
  host and configured GitLab Server base path; repository selection still controls
  Orkest-managed checkout.

### 3.3 Task

- Fields: title, description, stage, labels, user comments, repository links, timestamps.
- Optional agent-definition assignment. Deleting the definition clears assignment,
  preserving the task. Assignment never starts work; completion never selects another task.
- Manual content is locally managed. Imported title, description, labels, and URL
  are read-only and source-managed.
- Source closure moves tasks to the configured closed stage. Leaving the selected
  source/filters removes the imported task.
- Local workflow never updates source status; source comment sync remains open.

### 3.4 Integration

- GitHub Projects v2 and Jira Server poll into a selected workspace workflow.
- Providers own typed configuration, validation, and authenticated catalog schemas
  behind a common contract.
- Proposed contract: connectivity validation, discovery, field mapping, idempotent
  updates, missed-update reconciliation, and failure reporting.
- GitHub imports issues/draft issues, excluding PRs. [Label semantics](guides/integrations.md#task-integrations)
  differ by provider; no unified policy is established.

### 3.5 Tool

- Built-in Streamable HTTP MCP tools expose authorized task/repository data and artifacts.
- `get_task` / `list_workspace_repositories`: starting user/workspace scope.
- `list_artifacts` / `get_artifact`: task scope. `publish_artifact` also requires a producing run.
- Credentials are short-lived. Tools cannot bypass assignment, start, completion, or access policies.
- Custom tool registration, scripts, and catalogs are deferred. Runtime shell/file
  access remains permission-bound; external MCP servers are separate.

### 3.6 MCP Server

- Workspace fields: name and required immutable transport (`http` or `stdio`);
  omitted or empty transport is rejected. HTTP uses an
  HTTP(S) URL and optional encrypted/write-only headers. Stdio uses an absolute
  executable path, literal argument array, and optional encrypted/write-only environment.
- Agents select same-workspace servers. Deleting a server removes selections.
- Header/environment updates: omitted preserves; empty map clears. Arguments omitted
  preserve existing arguments; an empty array clears them. Credentials belong in headers
  or environment, never command arguments.
- Active processes retain startup settings. Continued sessions pin transport, endpoint
  or command/arguments, and stdio environment names; resolve current credential values
  by ID. Missing required names fail visibly. New sessions use current catalog settings.
- Runtimes launch stdio servers inside the execution environment using its existing
  task working directory. Install pinned server dependencies in execution images;
  automatic package installation and custom MCP working directories are excluded.
- OAuth setup remains deferred.

### 3.7 Agent Definition

- Fields: name, description, instructions, runtime, model provider/ID, permission
  policy, skills, MCP selections, optional runtime/execution profiles.
- Definitions supply configuration, not capacity. Versioning remains open.
- `ask` is the default; see §2 for permission policies and [configuration](guides/agent-configuration.md) for setup.
- Runtime profiles preset runtime/model/provider configuration. Updates change linked
  definitions atomically; deletion retains resolved values.
- Provider values are encrypted, write-only, and override matching workspace names
  for new sessions. Continued sessions retain compatible configuration.

### 3.8 Agent Team and Team Member

Confirmed capacity requirement; implementation unfinished:

- Each workspace team member selects an agent definition and supplies one execution slot.
  Multiple members may share a definition.
- Many tasks may be assigned; only one may execute per member. Reserve atomically.
- Input/approval waits occupy the slot. Release only after the step ends and ownership resolves.
- Completed idle conversations occupy no slot; revisions reacquire capacity.
- Assignment neither reserves capacity nor starts work.

### 3.9 Executor

- Docker: default isolated environment.
- Local: explicit trusted, single-user development option without task isolation.
- Contract: create, start, connect, inspect, stop, clean up.
- Kubernetes uses an operator-provisioned shared PVC and direct Pods; image-profile
  builds and full runtime/recovery qualification remain incomplete. See the
  [Kubernetes executor reference](reference/executors/kubernetes.md).

### 3.10 Workflow

- Tasks/integrations select a workspace board. Stages have stable IDs, names,
  descriptions, order, and optional responsible agents.
- Explicit initial/closed references determine entry/closure; display order does not.
  Occupied stages and referenced workflows cannot be deleted.
- Reopening remains unresolved; see [current behavior](guides/workflows.md) and
  [policy reconciliation](status.md#product-decisions-requiring-reconciliation).
- Confirmed automation: durable events trigger stage-entry starts and independent
  success/manual-completion movement rules. At most one enabled rule per event/source
  stage; no outgoing rules from closed stages.
- Rules continue only the same task. Failures, cancellation, and waits never advance/retry.
- Movement applies assignment atomically. Failed asynchronous starts expose failure
  without undoing movement; explicit starts remain separate.
- Configuration saves neither trigger work nor change active sessions. Evaluate
  events idempotently against current configuration; reject stale effects.
- Artifacts retain immutable revisions and optional run/stage-entry identity.
  Support text/JSON and external references. Required pinned inputs gate consumer
  startup; outputs do not gate producer completion.
- PR-triggered rules/provider conditions remain future work.

See [workflow behavior](guides/workflows.md) and [transactions](development/workflows.md).

### 3.11 Task Run

- One work step: effective configuration, environment, session, timestamps, events,
  approvals, outcome.
- Recovery retains the run; user-started revisions create another.
- Team-member linkage is unfinished. Final state taxonomy remains open;
  implemented values are in OpenAPI.

### 3.12 Agent Skill

- Globally unique lowercase ID; matching frontmatter `name`, nonempty `description`, Markdown body.
- Visibility/administration: global (platform admins), organization (Owners/Admins),
  workspace (Admins/inherited administrators).
- Definitions select visible skills; runs snapshot Markdown instructions.
- Companion files are deferred. Future support must preserve the primary Markdown
  field's meaning. See [native installation](reference/runtimes/README.md).

### 3.11 Scheduled jobs

Confirmed feature scope:

- A scheduled job is repeatable agent work independent of workflow tasks and stages.
- Instructions, agent definition, selected resources and calendar determine each invocation.
- Enabling explicitly authorizes future invocations; assignment alone still does not start board tasks.
- Each occurrence has an independent durable session/run, activity, approvals and result.
- Jobs may review repositories, update documentation/open merge requests, or use
  existing external tools such as an authorized email connector.
- Workspace scope and existing viewer restrictions apply to definitions, runs and interactions.

Initial implementation choices: daily/weekly calendars plus standard five-field
crontab expressions in an IANA time zone; explicit/all/absent registered
repositories; one-hour lateness/capacity grace; no overlap per job; and one active
scheduled execution per workspace. Cron accepts minute, hour, day-of-month, month,
and weekday fields with lists, ranges, steps, month/day names, and Sunday as 0 or 7.
Agent definitions do not represent member slots. Approval waits and uncertain
ownership retain job capacity. Future occurrences use fresh configuration;
accepted runs retain compatible snapshots.
These choices are documented in [scheduled jobs](development/scheduled-jobs.md).

## 4. Execution and communication

The platform launches independent agentctl workers. Workers own runtime processes
and ACP stdio; platform communication uses REST commands and a receive-only
activity WebSocket. See [architecture](architecture.md#system-boundary) for the
system diagram and [status](status.md) for transport limitations.

Required user-facing capabilities:

- View task execution state and agent activity.
- Chat with the agent in the context of a task.
- Inspect and answer approval requests.
- Inspect and answer structured clarification questions in task and direct
  conversations, bound to their original run/session/invocation. Question waits
  retain execution ownership; permission policy never supplies an answer.
- Cancel execution and explicitly resume eligible queued work; see §4.2.
- Inspect failures and results without losing prior conversation or execution history.
- Start workspace-scoped conversations directly with agent definitions: task-chat
  session/interaction behavior, without a task, initial task-context prompt, or workflow change.

### 4.1 Runtime selection

**RUNTIME-01:** Runtime, model provider, and model identifier are distinct settings.
Use an explicit model; never silently substitute a paid or different model.
Pin each runtime and validate instructions, skills, HTTP MCP, approvals,
cancellation, durable storage, and restore followed by another user message.

**RUNTIME-02:** Bind effective runtime/model configuration and permission policy
to the session. Changed definitions apply to new sessions. Incompatible restore
must preserve the saved state and report the reason, without a fresh replacement.

Provider-specific credentials and native permission modes are documented in
[runtime setup](reference/runtimes/README.md). Live qualification remains required.

### 4.2 Task sessions — confirmed interaction decisions

- One current conversation per task; older sessions may remain as history.
- Explicit starts, including messages from review, choose continue or new session.
- Continued sessions keep original configuration; new sessions use the current assigned agent.
- Follow-ups queue durably in order. Failure/cancellation pauses the queue until user resume/discard.
- Infrastructure recovery restores/continues only after safe reconciliation;
  uncertain effects require intervention.

See [session behavior and verification limits](guides/sessions.md).

## 5. Proposed manual execution flow

This earlier member-based proposal differs from current assignment, repository
selection, and workflow automation. It remains a proposal; see [reconciliation](status.md#product-decisions-requiring-reconciliation).

1. A user creates/imports a task and assigns a team member. The agent determines
   required repositories from workspace context; see the unresolved selection policy.
2. The user moves the task into in progress. The platform validates access,
   repositories, and availability, then reserves the member's slot atomically.
3. The executor creates/restores the environment and selected checkouts, preserving
   existing branches. The agent creates task branches only where changes are needed.
4. agentctl establishes ACP; the platform supplies task context. The user monitors,
   chats, and answers approvals while the agent performs permitted SDLC actions,
   including creating/updating PRs.
5. The platform records the outcome and releases capacity after execution and
   ownership are resolved. Success moves to review. User-started revisions
   reacquire capacity and reuse branches and context.

Imported content remains source-managed; local workflow never changes source
status. A failed launch exposes an error and releases its reservation only after
execution is confirmed absent. Ordinary failures never auto-retry.

## 6. Isolation, credentials, and access

- Authorize task APIs, events, chat, artifacts, integrations, and execution controls.
- Prevent cross-organization access through IDs, subscriptions, or executor connections.
- Grant environments only task-required configuration and credentials.
- Store/transmit credentials securely; redact ordinary logs.
- Encrypt workspace secrets and session environment snapshots at rest. Never return
  secret values through workspace APIs or ordinary logs.
- Bind approvals to task/run/session; stale responses cannot authorize another action.
- Proposed: audit assignments, transitions, execution controls, and approval decisions.

## 7. Monitoring and reliability

- Required: active runs, member availability, execution failures, integration sync failures.
- Proposed: persistent events, connection status, restart reconciliation, lost-environment detection.
- Confirmed recovery rules follow.

### 7.1 Confirmed recovery requirements

- Platform stops: agents continue independently; restart reconnects existing executions.
- Executor/agent stops: preserve progress and continue from the last recoverable state.
- Disconnection/recovery must neither duplicate execution nor free a member while
  its previous task might still run.

### 7.2 Confirmed recovery design

**RECOVERY-01 — Independent lifecycle:** agentctl owns runtime stdio and keeps
working during platform downtime. Journal ordered events and pending approvals;
reconnect by saved identity and cursor. Disconnects never release capacity.
Unavailable platform tools wait or report failure; never fabricate success.

**RECOVERY-02 — Durable state:** retain working trees, untracked/uncommitted work,
branches, runtime homes, session configuration, paths, events, and pending interactions
outside disposable layers. Restore compatible versions at the same paths.

**RECOVERY-03 — Honest continuation:** restore from the last durable checkpoint.
An in-flight model request, shell command, or process instruction cannot be resumed
exactly by assumption. Loading conversation history is insufficient. Validate each
adapter's continuation; report lost or uncertain progress. Failed/incompatible
restore preserves the session. A new conversation is an explicit user action.

**RECOVERY-04 — Single owner:** retain run/command identities, durable acceptance
and results, and exclusive ownership. Confirm termination or revoke the old
owner before replacement. Heartbeat expiry is insufficient. Reconcile uncertain
external side effects before retry; cross-system exactly-once is not assumed.

**RECOVERY-05 — Bound approvals:** persist request and response delivery identity.
Reconcile after restore; never replay a response into a different request or
approve automatically. Approval-required work waits for a user.

**RECOVERY-06 — Scope:** cover platform/process/container restart with durable
storage intact. Host/disk-loss recovery and backup objectives are deferred.
Recovery retains the logical run and records attempts/process identities. A
user revision creates a new run. Intentional cancellation never auto-resumes.

### 7.3 Recovery acceptance scenarios

| Scenario | Required result |
| --- | --- |
| Platform restarts during work | Agent continues; ordered activity reconnects without duplicate prompts/runs |
| Platform restarts during approval | Request stays pending; user can answer once through the reconnected UI |
| Executor restarts with uncommitted/untracked files | Restore files, branches, and supported session without restarting from scratch |
| Connection lost during command acceptance | Reconcile command identity before resend/replacement |
| External effect interrupted | Reconcile uncertain outcome or request user intervention before retry |
| Restore fails or durable storage is lost | Expose interruption/recovery limits; never claim successful resume |

Retention, recovery-time, host-loss, and availability targets remain open.

## 8. Proposed MVP acceptance criteria

- **MVP-01:** Users authenticate through Keycloak and access organizations/workspaces subject to application membership and role permissions.
- **MVP-02:** A failed/interrupted execution is visible while its task remains in progress; a normal task failure does not automatically start another attempt.
- **MVP-03:** OpenCode, Claude Code, Codex, GitHub Copilot CLI, and Kiro CLI execute through agentctl with configured model selection, HTTP MCP connections, and task-specific approvals.
- **MVP-04:** Built-in platform tools expose only authorized workspace task and repository information; no custom tool catalog is required.
- **MVP-05:** The skill catalog is separate from the deferred custom tool catalog. Skill IDs are globally unique to prevent collisions across scopes. Skill frontmatter requires a matching `name`, a non-empty `description`, and a Markdown instruction body.
- **MVP-06:** A user can register repositories, create an agent definition, and add two team members using that definition.
- **MVP-07:** Both members can execute separate tasks concurrently; one member cannot execute two tasks concurrently.
- **MVP-08:** Assignment alone does not start execution. An explicit user transition into in progress initiates execution when prerequisites are satisfied.
- **MVP-09:** A task can use multiple repositories registered in its workspace and executes in an isolated Docker environment.
- **MVP-10:** A user can observe activity, send a task-specific message, and answer an approval request through the platform and agentctl.
- **MVP-11:** A member can have multiple assigned tasks; an unfinished step, including approval waits, occupies its slot.
- **MVP-12:** Successful agent completion moves the task into review and releases the slot; starting revisions requires a user action in the UI.
- **MVP-13:** The agent identifies relevant workspace repositories and uses a task-specific branch in each changed repository.
- **MVP-14:** Local workflow changes do not update source status.
- **MVP-15:** Agents create and update pull requests autonomously; users do not need to perform these actions on the agent’s behalf.
- **MVP-16:** Users mark manually managed tasks done; closing an imported source issue also moves its Orkest task to done. The initial manual task API does not support reopening done tasks.
- **MVP-17:** All project configuration is workspace-scoped; configuration and credentials from another workspace cannot be selected or used implicitly.
- **MVP-18:** GitHub Projects v2 import and source updates preserve source identity and avoid duplicate tasks when updates are delivered repeatedly.
- **MVP-19:** Source-managed task fields are read-only in Orkest.
- **MVP-20:** Failed launches and interrupted sessions remain visible and do not silently erase run history or create duplicate execution.
- **MVP-21:** Executor and integration implementations are accessed through common contracts rather than provider-specific orchestration branches.

## 9. Remaining validation priorities

1. Qualify pinned runtimes with real approvals and restore followed by a message.
2. Implement team-member capacity allocation and verify concurrent reservations.
3. Define transport authentication and deployment resource/network policy.

## 10. Open decisions

- Resolve [policy differences](status.md#product-decisions-requiring-reconciliation)
  in starts, permissions, reopening, assignment, and repository selection.
- Define base-branch overrides and further pull-request policy. New checkouts
  currently use discovered default branches; existing checkouts retain branches.
- Define agent-definition versioning and final run-state taxonomy.
- Define invitations and external membership provisioning beyond first login.
- Define source comment synchronization and outbound writes; polling, source
  closure, and selected-source removal already have documented behavior.
- Define resource limits, network policy, cleanup, retention, audit, and cost reporting.
- Define backup/recovery objectives; host/disk-loss recovery remains deferred.
- Define external workflow event conditions and artifact representations beyond
  stored text/JSON and external references.

## 11. Execution toolchain profiles

Confirmed:

- Multiple workspace profiles pair supported runtimes with shared or separate toolchain images.
- Create/update queues asynchronous composition and registry publishing; explicit
  profile revisions are unnecessary.
- Ready images never interrupt active work/approval waits. The next user-started
  run may restore the session with the new image.
- Build attempts and run image identities retain provenance.

See [execution profiles](guides/execution-profiles.md) for roles, runtime-change
constraints, and operational qualifications.
