# Orkest requirements

Status: Draft for discussion. Confirmed requirements are distinguished from proposals and open decisions below.

## 1. Purpose

Orkest is a platform for orchestrating, monitoring, and interacting with AI agents, primarily for software development lifecycle (SDLC) tasks. The platform launches agents in isolated task environments and supports human-directed execution and interaction through Agent Client Protocol (ACP).

## 2. Confirmed MVP scope

- Support multiple organizations, users, and workspaces from day one.
- Delegate authentication to Keycloak to support username/password and SSO through identity-provider configuration.
- Use organization roles Owner/Admin/Member and workspace roles Admin/Member/Viewer.
- Use OpenCode as the initial agent runtime; keep runtime adapters extensible.
- Provide built-in tools for communicating with the core platform; user-defined tools are outside MVP scope.
- Use the pinned OpenCode release’s default permission behavior. Forward any runtime approval requests to the task UI; workspace permission-policy customization is deferred.
- Support manual task creation and tasks imported from GitHub Projects v2 issue and draft issue items.
- Require users to assign tasks to specific team members and manually move tasks to in progress.
- Support reusable agent definitions and workspace teams with individual members providing execution slots.
- Execute agents using Docker behind an executor abstraction suitable for future implementations such as Kubernetes.
- Build agentctl as the bridge between the platform and agent ACP sessions in execution environments.
- Support task-specific chat, agent approval requests, cancellation, and recovery after platform or executor interruption. Recovery proposals and provider limitations are described in section 7.
- Support HTTP MCP servers; stdio MCP servers are outside the MVP.
- Start with a simple workflow: todo, in progress, in review, done.
- Allow a task to involve multiple repositories registered in its workspace.
- Treat external systems as authoritative for imported task content; users edit that content at the source.

Automatic assignment and automatic movement into in progress are outside the MVP. OpenCode is the confirmed MVP runtime. Specific model, deployment, and technology choices remain open.

## 3. Domain model

### 3.1 Organization, User, Workspace

An organization groups users and multiple workspaces. Each workspace represents an isolated project within the organization. Repositories, tasks, agent definitions, team members, tools, MCP servers, integrations, executor configurations, and workflow configuration belong to a workspace and are isolated from other workspaces, including those in the same organization. Configuration and credentials must not be implicitly shared across projects.

Workspace Admins and inherited organization Owners/Admins can manage workspace environment variables and secrets. Variables are readable through workspace configuration APIs; secret values are encrypted at rest and write-only. Workspace values are supplied to the runtime process and child tools for each run. Updates apply to future runs; an existing run's environment snapshot is encrypted and retained for recovery.

Organization roles are Owner, Admin, and Member. Workspace roles are Admin, Member, and Viewer. Authentication is delegated to Keycloak. Orkest owns user profiles, organization/workspace membership, and application authorization; Keycloak authenticates users. On first successful login, Orkest creates a user keyed by the verified Keycloak subject and records username and email. The profile is not overwritten by later logins. Users must be organization members before receiving workspace membership.

Organization Owners manage the organization and all memberships. Organization Admins manage organization settings and workspaces and manage memberships, but cannot grant, change, or remove an Owner. Organization Members can read organization details. Owner and Admin access is inherited across every workspace in the organization. Workspace Admins manage workspace settings and memberships; Members may read and edit project resources; Viewers are read-only. The last organization Owner cannot be demoted or removed. Invitations are not part of this first membership API. A workspace Viewer must not mutate tasks or configurations, start executions, or answer approval requests. The platform must enforce organization and workspace access boundaries on all data and execution operations.

### 3.2 Repository

Users register repositories in a workspace before tasks may use them. A task may involve one or more registered repositories. A task must not access repositories from another workspace through task configuration.

The GitHub coding provider contract and encrypted coding integration persistence
are under implementation. The current accepted provider is GitHub Cloud with a
PAT; additional coding providers remain future support. Repository discovery,
integration management APIs, authenticated executor credential resolution, and
the integration-based repository picker are not yet implemented. Repository
credentials must remain write-only and separate from repository metadata and
run configuration. Git access tokens are not passed to agents in this slice.

Repository configuration must identify the remote repository and the credentials or integration used to access it. The agent determines which workspace repositories need changes from the task context. Each changed repository uses a branch dedicated to the task; review iterations reuse that branch. Agents autonomously create and update pull requests and perform the repository operations needed to complete the task, within configured permissions and approval policies. Base branch selection and discovery mechanics remain open.

### 3.3 Task

A task contains a title, description, workflow status, freeform labels, comments, components, and timestamps. It belongs to a workspace and may be assigned to a workspace agent definition. Manual task management initially exposes workspace-scoped APIs; task UI and execution wiring are separate work.

A component is a workspace-scoped entity with a user-supplied identifier unique within the workspace, a name, description, and one associated repository registered in that workspace. Tasks may reference multiple components. Identifiers serve as stable human-readable keys, similar to labels.

Deleting an assigned agent definition clears that task assignment while preserving the task. Task comment authors remain users.

Tasks may be created in Orkest or imported from an integration. Imported tasks retain their source identity and a link to the source. Source-managed fields cannot be edited in Orkest.

Users assign tasks to workspace agents. Only a user can initiate work by moving a task into in progress. Linking execution slots/team members to these agents is part of the team and orchestration work. Agent completion must not automatically initiate another task.

Orkest manages its own assignment, execution, and agent chat without updating source status. Imported title, description, labels, and source URL remain source-managed. When a source issue is closed, the imported task moves to done; when an item is removed from an integration's selected source or no longer matches its filters, its imported task is removed from Orkest. Source comment synchronization remains open.

### 3.4 Integration

An integration connects a workspace to an external task system, imports tasks, and keeps source-managed task data synchronized. GitHub Cloud Projects v2 and Jira Server are the initial implementations. Each source is configured and enabled per workspace, selects source tasks using label filters, and is polled on a configurable interval. GitHub Cloud sources select a Projects v2 board URL; Jira sources select project keys on their configured server.

Integration implementations must conform to a common contract so another task system can be added through a new implementation class without changing the core task or orchestration logic. Each provider owns a typed configuration, strict validation, and a machine-readable config schema. The authenticated provider catalog exposes these schemas to setup clients. Provider configuration, credentials, and source-specific mapping may differ.

Proposed contract responsibilities:

- Validate configuration and connectivity.
- Discover or import supported source tasks.
- Map source content into the internal representation while keeping local workflow status separate from source status.
- Process source updates idempotently.
- Reconcile missed updates and surface synchronization failures.

MVP does not change source status. Imported title, description, labels, and source URL are read-only in Orkest and updated idempotently by polling. Closed source issues move their imported task to done. Items removed from the selected source or no longer matching its filters are deleted from Orkest. GitHub Cloud imports issue and draft issue items from the selected board; pull request items are excluded. Label filters require all configured labels on an item. Credentials are encrypted at rest and omitted from API responses and ordinary logs. Outbound comments and other source writes remain unspecified. Local task workflow status, source status, and run execution state are separate concepts.

### 3.5 Tool

MVP tools are built-in capabilities that let an agent communicate with the core platform. Initial capabilities include listing tasks, getting task details, and retrieving workspace repository information. Calls must be authenticated and scoped to the agent's workspace and allowed operations.

User-defined tool registration, custom scripts/commands as platform tools, and a configurable Tool catalog are outside the MVP. External MCP servers remain separately configurable; native runtime capabilities such as shell execution and file editing remain available subject to permissions.

The exposure mechanism for internal tools remains an implementation decision; an internal HTTP MCP endpoint is a candidate, not a confirmed requirement. Agents must not use internal tools to bypass manual assignment/start controls or user-only completion and reopening rules.

### 3.6 MCP Server

An MCP server configuration represents a workspace-scoped HTTP connection to capabilities provided by an external server, such as GitHub or Jira. MVP supports OpenCode's remote Streamable HTTP transport; stdio servers and OAuth setup are outside MVP scope. Agent definitions select zero or more MCP servers registered in the same workspace.

Configuration includes a display name, absolute HTTP(S) endpoint URL, and optional custom request headers. Header values are encrypted at rest using the integration encryption key, write-only in APIs, and omitted from ordinary logs. Updates that omit headers preserve stored values; an explicitly supplied empty header map clears them. At launch, only the selected servers and decrypted headers are passed to agentctl and configured in that run's OpenCode runtime. Server deletion removes its agent selections. MCP server changes affect future runs; already-running processes keep their startup configuration.

### 3.7 Agent Definition

An agent definition is reusable configuration containing:

- Name and short description.
- System prompt / agent declaration.
- Agent runtime: OpenCode for MVP, through an extensible runtime adapter.
- Model provider and model identifier supported by that runtime.
- Model selection.
- Selected MCP servers. Built-in platform tools are provided by the platform; a custom tool selection UI is outside MVP.

An agent definition belongs to a workspace and does not itself supply execution capacity. Its versioning policy remains open.

### 3.8 Agent Team and Team Member

A workspace has an agent team. Users add members to the team and select an agent definition for each member. Multiple members may reuse the same definition.

Each member provides one execution slot and may work on at most one task at a time. The slot becomes available when the member is no longer working, including when a task awaits review after execution completes.

A member may have multiple tasks assigned but execute only one task at a time. Assignment alone does not occupy a slot or start work.

Slot allocation must be atomic so concurrent user actions cannot launch two tasks on one member. A slot remains occupied until the current work step/session finishes, including while waiting for user input or approval. An open but idle conversation after a completed step does not occupy a slot. Restarting work on an assigned task must acquire the member’s slot again.

### 3.9 Executor

An executor provisions and manages a task execution environment. Docker is the first implementation; orchestration must use an abstraction that can later support Kubernetes and other executors.

The executor contract must cover environment creation, startup, connectivity, status inspection, stopping, and cleanup. Task environments must be isolated from other tasks. Resource limits, network policy, persistence, and cleanup timing remain open.

### 3.10 Workflow

The initial workflow consists of todo → in progress → in review → done. Assignment and movement into in progress require explicit user actions. Workflow stages are fixed for the initial task API. Done tasks cannot be reopened in this initial slice.

Successful agent completion moves the task into review and releases its slot. Starting another work step after review requires an explicit user action in the platform UI and moves the task back into progress. Only users can mark tasks done. Reopening done tasks is not supported in the initial task API. A failed or interrupted execution leaves its task in progress with a visible failed/interrupted execution indicator until a user retries or changes its status. Infrastructure recovery may continue the existing interrupted work step according to section 7; ordinary task failure does not trigger an automatic new attempt. A workflow status is distinct from execution health.

### 3.11 Task Run — proposed supporting concept

A task run records one execution attempt for a task. It links the task, assigned team member, effective agent definition, executor environment, and ACP session.

It preserves timestamps, execution state, events, logs, approval requests, and results so retries do not erase history. Proposed states include starting, running, waiting for input, succeeded, failed, cancelled, and interrupted; exact states remain open.

The platform should snapshot effective configuration for each run so later edits to an agent definition do not change the historical record.

### 3.12 Agent Skill

An agent skill is a reusable `SKILL.md` instruction document in the Agent Skills format. Skills have global scope (shared across organizations and workspaces), organization scope, or workspace scope. Users can read skills within their visibility scope; platform admins manage global skills, organization owners/admins manage organization skills, and workspace admins plus inherited organization owners/admins manage workspace skills. These roles can create, update, and delete skills. Global platform-admin status is application-managed on the user record and is provisioned by deployment operators.

Agent definitions select skills by their globally unique lowercase skill ID. The API and run configuration use `skillMD` for the primary Markdown file. At task start, the platform resolves only skills visible in that workspace and snapshots `skillMD` into the run configuration. OpenCode receives the selected `SKILL.md` files under its project skill directory, where its native skill tool can discover and load them. V1 does not support companion files; the catalog can later extend from `skillMD` to the full directory structure without changing the meaning of this field.

## 4. Execution and communication

Proposed communication structure:

```text
User → Orkest → Executor-managed environment
                 └─ agentctl ↔ Agent (ACP)
```

The platform launches the agent. For Docker and future remote execution environments, agentctl bridges platform communication to the agent's ACP session. The platform-to-agentctl transport, authentication, reconnect behavior, and ownership of process startup remain open; ACP does not by itself settle that transport design.

Required user-facing capabilities:

- View task execution state and agent activity.
- Chat with the agent in the context of a task.
- Inspect and answer approval requests.
- Cancel execution and resume interrupted work, with precise semantics still to be defined.
- Inspect failures and results without losing prior conversation or execution history.

### 4.1 OpenCode runtime requirements

OpenCode is the sole initially supported runtime. The platform must distinguish the agent runtime from the model provider and model identifier. Model selection remains configurable on agent definitions, allowing available free models without depending on a permanently free hardcoded model.

The official [OpenCode ACP reference](https://dev.opencode.ai/v2/docs/cli/acp/) documents `opencode acp` as a child process using ACP over stdio, plus session load/resume and HTTP MCP support. agentctl owns this local subprocess connection and exposes the separate platform-facing transport. The HTTP-only restriction applies to MCP connections, not the local ACP transport.

Use the pinned release’s default permissions without introducing a custom workspace policy editor or blanket automatic approval. If the runtime requests permission, the user approves or rejects it through the task UI and the slot remains occupied while waiting. Record the effective runtime version and permission configuration with each run.

Pin and validate the selected OpenCode release before implementation acceptance: verify model selection, system prompt configuration, HTTP MCP headers, approval round trips, durable session storage, and continuation after process restart. Document and persist the required OpenCode data directories and restore the same working paths. Documentation support for session restoration is not a substitute for testing interrupted-work recovery.

OpenCode documents free model offerings, some available for a limited time; available models and credentials must be configuration rather than immutable requirements. See [OpenCode models](https://dev.opencode.ai/v2/docs/console/models/). Do not silently select a paid model if a configured free model becomes unavailable.

### 4.2 Task sessions — confirmed interaction decisions

Tasks retain one current agent conversation across review iterations and may keep
older sessions as history. When explicitly starting another work step, users
choose to continue that session or create a new one. Continued sessions retain
their original configuration; new sessions use the current assigned agent.
Sending a message from review is an explicit start action with that same choice.
Follow-up prompts during work queue durably and execute in order. Failure or
cancellation pauses the queue until a user resumes or discards it. Infrastructure
recovery automatically restores and continues where reconciliation establishes
that doing so is safe; uncertain effects require intervention. See the
[implemented design and verification limits](task-sessions.md).

## 5. Proposed manual execution flow

1. A user creates a task or an integration imports it.
2. A user assigns a specific team member. The agent receives workspace repository context and determines which repositories the task requires.
3. A user explicitly moves the task into in progress.
4. The platform validates access, repository configuration, and member availability, then reserves the member's slot atomically.
5. The executor creates or restores the isolated task environment. Repository preparation supports discovery across workspace repositories and a branch per task in each changed repository.
6. agentctl establishes the agent ACP session and the platform provides task context.
7. The user monitors execution, chats, and responds to approval requests.
8. The platform records the outcome and releases the slot when execution ends.
9. The agent autonomously creates or updates pull requests and performs other task-required SDLC actions within its configured permissions. Successful agent completion moves the task into review. A user may explicitly start another work step from the platform UI; this reacquires the member slot and reuses task branches and preserved context.

Imported tasks use local assignment and execution controls without changing source status. A failed launch must release its reservation only once execution is confirmed absent, and expose a useful error. Automatic retries for task failures are not yet specified; infrastructure recovery is described below.

## 6. Isolation, credentials, and access

- Authorization must apply to task APIs, events, chat, artifacts, integrations, and execution controls.
- One organization's users must not gain access to another organization's resources through identifiers, subscriptions, or executor connections.
- Execution environments must receive only the configuration and credentials needed for the task.
- Credentials must be stored and transmitted securely and redacted from ordinary logs.
- Workspace secrets and task-session environment snapshots must be encrypted at rest; secret values must not be returned by workspace APIs or written to ordinary logs.
- Approval requests must remain associated with the correct task, run, and session; stale responses must not approve a different action.
- Proposed: record an audit trail for assignments, transitions, execution controls, and approval decisions.

## 7. Monitoring and reliability

The platform must expose active task runs, member availability, run failures, and integration synchronization failures. Proposed operational requirements include persistent execution events, connection status, restart reconciliation, and detection of lost environments.

### 7.1 Confirmed recovery requirements

- If the core platform stops, agents continue working independently. When the platform restarts it reconnects to the existing executions.
- If an executor or agent process stops, recovery must preserve task progress and continue from the last recoverable state.
- A disconnected or recovering task must not cause duplicate execution or allow its member to execute a second task while the first might still be running.

### 7.2 Confirmed recovery design

**Core platform restart:** agentctl owns the local agent process and ACP connection independently of the core platform lifecycle. It durably records ordered events and pending approval requests, continues active work while disconnected, and pauses at any approval requiring a user response. On reconnect, the platform reconciles execution identity and state and requests events after its last acknowledged cursor. Disconnection alone neither cancels a run nor releases its slot. Work requiring unavailable platform tools waits or reports the dependency failure; it must not fabricate success.

**Executor or agent restart:** task data resides on durable storage outside the disposable container layer. Persist repository working trees (including uncommitted and untracked work), task branches, agent session storage, runtime configuration identity, working paths, event history, and pending interaction metadata. agentctl restores the same paths and compatible runtime, loads or resumes the persisted session when supported, reconciles the last operation, and continues the interrupted work step. Recovery continues existing work and does not authorize a new step after review.

**Recovery guarantee:** preserve durable files and recorded session context, then continue from the last durable checkpoint. An arbitrary process instruction, in-flight model request, or unfinished shell command cannot be guaranteed to resume exactly. Session reload restores conversation context; it does not alone prove interrupted work is still running or automatically continues. Each runtime adapter must implement and validate continuation behavior. Recovery must report lost or uncertain progress rather than claim exact restoration.

**Runtime eligibility:** require durable session restoration for initially supported runtimes. If unsupported or restoration fails, keep the task interrupted with a visible reason. Starting a new session with reconstructed context is a separate, explicit user action, not a silent substitute for session recovery. ACP session restoration methods and capabilities vary by protocol version and agent implementation; pin and test the supported combination. See the official [ACP v1 session setup](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/session-setup.mdx) and [ACP v2 session setup](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v2/session-setup.mdx).

**Duplicate prevention:** use stable run and command identities, durable command acceptance/results, and one active execution owner per run. Reconnection or an expired heartbeat is not authorization to create a replacement. Before replacement, confirm the previous execution has stopped or revoke its ability to act. Reconcile uncertain external side effects, such as creating a pull request, before retrying them; generic exactly-once execution across external systems is not assumed.

**Approvals:** persist request identity and response delivery state. After session restoration, reconcile requests with the agent; do not replay a response into a new or different request. Never automatically approve a request to unblock recovery.

**Storage scope:** Docker volumes can cover container recreation on the same host if retained. Host or disk loss requires separately durable storage or backups and an explicit recovery-point objective. The MVP guarantee covers platform/process/container restarts with durable task storage intact. Recovery from complete executor-host or disk loss is deferred; backup objectives remain open.

**Run history:** infrastructure recovery retains the logical run and records recovery attempts and new process/session identities. A user-initiated revision after review creates a new work step/run linked to the same task workspace and branches. Intentional cancellation is distinct from infrastructure failure and must not automatically resume.

### 7.3 Recovery acceptance scenarios

- Restart the core platform during active work: the agent continues; reconnect restores ordered activity without duplicate prompts or runs.
- Restart the platform while approval is pending: the request remains pending and the user can answer it once through the reconnected UI.
- Restart an executor with uncommitted and untracked files: restore those files, task branches, and the supported agent session without restarting the task from scratch.
- Lose a connection while a command is being accepted: reconcile the command identity before resending or launching a replacement.
- Interrupt an external side effect: detect an uncertain outcome and reconcile or request user intervention rather than blindly repeating it.
- Fail session restoration or lose durable storage: surface the interruption and recovery limitation without claiming successful resume.

Event retention, recovery time targets, host-loss guarantees, and overall availability targets remain open.

## 8. Proposed MVP acceptance criteria

- Users authenticate through Keycloak and access organizations/workspaces subject to application membership and role permissions.
- A failed/interrupted execution is visible while its task remains in progress; a normal task failure does not automatically start another attempt.
- OpenCode executes through agentctl with configurable model selection, HTTP MCP connections, and task-specific approvals.
- Built-in platform tools expose only authorized workspace task and repository information; no custom tool catalog is required.
- The skill catalog is separate from the deferred custom tool catalog. Skill IDs are globally unique to prevent collisions across scopes. Skill frontmatter requires a matching `name`, a non-empty `description`, and a Markdown instruction body.
- A user can register repositories, create an agent definition, and add two team members using that definition.
- Both members can execute separate tasks concurrently; one member cannot execute two tasks concurrently.
- Assignment alone does not start execution. An explicit user transition into in progress initiates execution when prerequisites are satisfied.
- A task can use multiple repositories registered in its workspace and executes in an isolated Docker environment.
- A user can observe activity, send a task-specific message, and answer an approval request through the platform and agentctl.
- A member can have multiple assigned tasks; an unfinished step, including approval waits, occupies its slot.
- Successful agent completion moves the task into review and releases the slot; starting revisions requires a user action in the UI.
- The agent identifies relevant workspace repositories and uses a task-specific branch in each changed repository.
- Local workflow changes do not update source status.
- Agents create and update pull requests autonomously; users do not need to perform these actions on the agent’s behalf.
- Users mark manually managed tasks done; closing an imported source issue also moves its Orkest task to done. The initial manual task API does not support reopening done tasks.
- All project configuration is workspace-scoped; configuration and credentials from another workspace cannot be selected or used implicitly.
- GitHub Projects v2 import and source updates preserve source identity and avoid duplicate tasks when updates are delivered repeatedly.
- Source-managed task fields are read-only in Orkest.
- Failed launches and interrupted sessions remain visible and do not silently erase run history or create duplicate execution.
- Executor and integration implementations are accessed through common contracts rather than provider-specific orchestration branches.

## 9. Suggested requirements completion order

1. Pin OpenCode and validate durable session restoration and interrupted-work continuation.
2. Define repository discovery, base branches, and pull request policy.
3. Define membership roles, configuration ownership, and credential access.
4. Specify tools, provider capabilities, and the agentctl boundary.
5. Finalize acceptance criteria and explicitly defer remaining features.

## 10. Open decisions

### Highest priority

1. **Repositories:** Define repository discovery/preparation and default base branches.
2. **Workflow details:** Define the destination status when a user reopens a task. Failed/interrupted executions retain in-progress task status with a visible execution indicator.

### Subsequent decisions

- Invitations and external identity-to-membership provisioning beyond first-login user creation.
- Custom workspace approval policies are deferred. MVP uses OpenCode defaults and forwards runtime approval requests without automatically accepting them.
- Pinned OpenCode release, default model selection, and validated recovery behavior.
- Whether workflows are fixed for MVP or configurable per workspace.
- Internal platform tool exposure mechanism and exact allowed operations; custom tool registration is deferred.
- GitHub import scope, comment synchronization, webhook/polling behavior, and issue-close semantics.
- Docker deployment location, environment persistence, resource limits, and credential provisioning.
- Artifact types, log/chat retention, audit requirements, and usage/cost reporting scope.
