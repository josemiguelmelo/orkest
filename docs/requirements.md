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
- Support manual task creation and tasks imported from GitHub Issues.
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

Organization roles are Owner, Admin, and Member. Workspace roles are Admin, Member, and Viewer. Authentication is delegated to Keycloak. Orkest owns organization/workspace membership and application authorization; Keycloak authenticates users. The detailed permission matrix, inheritance of organization privileges, invitations, and identity-to-membership provisioning remain to be specified. A workspace Viewer must not mutate tasks or configurations, start executions, or answer approval requests. The platform must enforce organization and workspace access boundaries on all data and execution operations.

### 3.2 Repository

Users register repositories in a workspace before tasks may use them. A task may involve one or more registered repositories. A task must not access repositories from another workspace through task configuration.

Repository configuration must identify the remote repository and the credentials or integration used to access it. The agent determines which workspace repositories need changes from the task context. Each changed repository uses a branch dedicated to the task; review iterations reuse that branch. Agents autonomously create and update pull requests and perform the repository operations needed to complete the task, within configured permissions and approval policies. Base branch selection and discovery mechanics remain open.

### 3.3 Task

A task contains a title, description, workflow status, and comments. It belongs to a workspace and may reference workspace repositories and an assigned team member.

Tasks may be created in Orkest or imported from an integration. Imported tasks retain their source identity and a link to the source. Source-managed fields cannot be edited in Orkest.

Only a user can assign a task to a specific team member or initiate work by moving it into in progress. Agent completion must not automatically initiate another task.

For MVP, assume source status does not change. Orkest manages its own workflow status, assignment, execution, and agent chat without updating the source status. Imported title and description remain source-managed. Source comment synchronization remains open.

### 3.4 Integration

An integration connects a workspace to an external task system, imports tasks, and keeps source-managed task data synchronized. GitHub Issues is the first implementation.

Integration implementations must conform to a common contract so another task system can be added through a new implementation class without changing the core task or orchestration logic. Provider configuration, credentials, and source-specific mapping may differ.

Proposed contract responsibilities:

- Validate configuration and connectivity.
- Discover or import supported source tasks.
- Map source content into the internal representation while keeping local workflow status separate from source status.
- Process source updates idempotently.
- Reconcile missed updates and surface synchronization failures.

MVP does not change source status and assumes no incoming source status changes. Outbound comments and other source writes remain unspecified. Local task workflow status, source status, and run execution state are separate concepts.

### 3.5 Tool

MVP tools are built-in capabilities that let an agent communicate with the core platform. Initial capabilities include listing tasks, getting task details, and retrieving workspace repository information. Calls must be authenticated and scoped to the agent's workspace and allowed operations.

User-defined tool registration, custom scripts/commands as platform tools, and a configurable Tool catalog are outside the MVP. External MCP servers remain separately configurable; native runtime capabilities such as shell execution and file editing remain available subject to permissions.

The exposure mechanism for internal tools remains an implementation decision; an internal HTTP MCP endpoint is a candidate, not a confirmed requirement. Agents must not use internal tools to bypass manual assignment/start controls or user-only completion and reopening rules.

### 3.6 MCP Server

An MCP server configuration represents an HTTP connection to capabilities provided by an external server, such as GitHub or Jira. Agent definitions select their MCP connections.

Configuration must support connection details and required authentication. Secrets must not be exposed in task content, chat, or ordinary logs. Exact HTTP transport support and credential distribution remain implementation decisions.

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

The initial workflow consists of todo → in progress → in review → done. Assignment and movement into in progress require explicit user actions.

Successful agent completion moves the task into review and releases its slot. Starting another work step after review requires an explicit user action in the platform UI and moves the task back into progress. Only users can mark tasks done or reopen them for MVP. Reopening does not itself start execution; work still requires an explicit user start action and an available member slot. The destination status when reopening remains open. A failed or interrupted execution leaves its task in progress with a visible failed/interrupted execution indicator until a user retries or changes its status. Infrastructure recovery may continue the existing interrupted work step according to section 7; ordinary task failure does not trigger an automatic new attempt. A workflow status is distinct from execution health.

### 3.11 Task Run — proposed supporting concept

A task run records one execution attempt for a task. It links the task, assigned team member, effective agent definition, executor environment, and ACP session.

It preserves timestamps, execution state, events, logs, approval requests, and results so retries do not erase history. Proposed states include starting, running, waiting for input, succeeded, failed, cancelled, and interrupted; exact states remain open.

The platform should snapshot effective configuration for each run so later edits to an agent definition do not change the historical record.

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
- Only users can mark tasks done or reopen them, and reopening alone does not launch execution.
- All project configuration is workspace-scoped; configuration and credentials from another workspace cannot be selected or used implicitly.
- GitHub Issues import and source updates preserve source identity and avoid duplicate tasks when updates are delivered repeatedly.
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

- Keycloak integration details, invitations, identity-to-membership provisioning, and detailed permissions for the confirmed roles.
- Custom workspace approval policies are deferred. MVP uses OpenCode defaults and forwards runtime approval requests without automatically accepting them.
- Pinned OpenCode release, default model selection, and validated recovery behavior.
- Whether workflows are fixed for MVP or configurable per workspace.
- Internal platform tool exposure mechanism and exact allowed operations; custom tool registration is deferred.
- GitHub import scope, comment synchronization, webhook/polling behavior, and issue-close semantics.
- Docker deployment location, environment persistence, resource limits, and credential provisioning.
- Artifact types, log/chat retention, audit requirements, and usage/cost reporting scope.
