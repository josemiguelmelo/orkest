# Docker agentctl executor

- One container per run; local Engine from `DOCKER_HOST` or default socket.
- Default endpoint: dynamic host port on `127.0.0.1`, container port 8081.
- `ORKEST_AGENTCTL_NETWORK`: join that network; endpoint is container name:8081.
  Compose uses shared `orkest` network for direct platform access.
- Return container ID/endpoint to launch service; names include runtime/run-ID hash for reconciliation.

## Build and configure images

From repository root:

```sh
make docker-agentctl-images
# Optional: publish all images to the local Compose registry.
make docker-agentctl-push-local
```

- Build target: base (agentctl, Git, provider CLIs) plus pinned runtime images, one
  runtime each. BuildKit runs on the platform. Pins: [inventory](../../../internal/contracts/runtimecatalog/versions.json).
- Push target starts registry, pushes to `localhost:${REGISTRY_PORT:-5001}/orkest`,
  tags Compose aliases under `registry:5000/orkest`.
- Registry overrides: `REGISTRY_PORT`, `LOCAL_REGISTRY`, `CONTAINER_REGISTRY`, `AGENTCTL_REGISTRY_NAMESPACE`.
- Base selector: `AGENTCTL_BASE_IMAGE`. Runtime selectors: `AGENTCTL_OPENCODE_IMAGE`,
  `AGENTCTL_CLAUDE_CODE_IMAGE`, `AGENTCTL_CODEX_IMAGE`, `AGENTCTL_GITHUB_COPILOT_IMAGE`, `AGENTCTL_KIRO_IMAGE`.
- Pass selected runtime to agentctl; reject mismatches.
- Rebuild after worker changes. Existing processes keep their binary; drain queues
  before adoption and preserve [session compatibility](../../guides/sessions.md).

## Durable files and credentials

- Scope: organization/workspace/task. The platform supplies
  `ORKEST_DATA_DIR/agent-executions/tasks/<scope-hash>`;
  hash hides identifiers from paths/container labels.
- Durable `work`/`agentctl-state` mount at `/workspace`/`/agentctl-state`.
  Directories: 0700; container UID/GID matches platform user for writable files.
- Selected repositories: `/workspace/repositories/<provider>/<repository-id>`.
  New checkout uses default branch; existing checkout retains branch/worktree across
  review iterations. Agents branch only for requested changes.
- Codex host login opt-in: `.env` `ORKEST_CODEX_AUTH_FILE` with absolute daemon-visible
  path, e.g. `/Users/alice/.codex/auth.json`. Codex-only read-only mount; agentctl copies
  to private Codex home, mode 0600. Never sent to other runtimes, database, or API.
- Unset auth file uses workspace `OPENAI_API_KEY`. Refreshes update only task copy;
  refresh host login if later authentication fails.
- [Coding integrations](../../guides/integrations.md) supply run-scoped Git/CLI credentials;
  see checkout wrappers, permissions, and public-repository limits.

## Workspace host volumes

Workspace settings expose **Docker host volumes**. Workspace Admins and inherited
organization Owners/Admins can add, edit, or remove bind mounts; other authorized
workspace members can view them. Configuration is entirely in the UI; no
environment variable or host-path allowlist is required.

1. Open workspace settings and select **Add volume**.
2. Enter an absolute host file/directory path and a container target such as
   `/mnt/host/project` or `~/.aws`.
3. Keep **Read only** selected, or deselect it to allow writes, then save.
   The selection is persisted in PostgreSQL and applies to new sessions without
   restarting the platform.

Sources are exact absolute, clean paths on the **Docker daemon host**, not paths
inside the platform container or automatically the browser's machine. Files and
directories are supported. Docker rejects missing sources; Orkest does not create
them. Docker Desktop/Colima must share the source with the VM. For a remote Engine,
provision files on that host. Source files need not be mounted into the platform
container. They must be accessible to its configured agent container UID/GID.

**Trusted deployments only:** workspace admins can select arbitrary host paths,
including files outside their workspace. Workspace authorization scopes the saved
settings, not access to the host filesystem. This configuration deliberately
trusts workspace admins with Docker host-file access and is not suitable for
mutually untrusted tenants. Do not mount another workspace's data or credentials.
At most 32 mounts are allowed, with nonoverlapping targets beneath `/mnt/host/`
or the private session home using `~/`. Targets must be clean paths without dot
segments or trailing slashes. Replacing runtime executables, `/workspace`,
`/agentctl-state`, or the whole home (`~` or `~/`) is not supported.
Verify the source and any symlinks; avoid the Docker socket, host roots, device
nodes, and unrelated credentials.
Do not share credential directories across workspaces. Bind mounts
deliberately expose host data to every new agent session in the selected workspace;
read/write mounts permit host-file modification and deletion.

The API is `GET`/`PUT /api/organizations/{orgID}/workspaces/{workspaceID}/host-mounts`.
`GET` returns `enabled` and `mounts`. `PUT` replaces
`mounts` with `{source, target, readOnly}` entries; `{"mounts":[]}` clears them.
Local and Kubernetes executors cannot use these mounts.

### Credential mounts

`/mnt/host` does not change a tool's default credential lookup. Use home-relative
targets for files expected under the agent's `HOME`, not `/root`:

| Host source example | Container target | Suggested access |
|---|---|---|
| `/Users/alice/.aws` | `~/.aws` | Read only; read/write if SSO cache refresh/login must persist |
| `/Users/alice/.m2/settings.xml` | `~/.m2/settings.xml` | Read only |
| `/Users/alice/.gradle/gradle.properties` | `~/.gradle/gradle.properties` | Read only |

Use your own absolute source paths. Tilde expansion applies only to targets.
The executor resolves `~/` against the same durable session home used by the
native runtime; the saved logical target stays unchanged across runs. Only
parent directories are prepared; credential contents are never copied. Mount
individual Maven/Gradle files instead of their entire directories so that local
caches remain writable. Home targets and their parent directories must not be
symlinks. Avoid replacing runtime-owned configuration directories
such as `.codex`, `.claude`, or `.config`.

AWS CLI discovers `$HOME/.aws` normally (unless overridden by tool configuration).
Java's `user.home` can differ from `HOME`; Orkest does not change JVM defaults or
inject environment variables. For predictable discovery without environment
configuration, invoke the tools with explicit paths:

```sh
aws configure list-profiles
./gradlew --gradle-user-home "$HOME/.gradle" build
mvn -s "$HOME/.m2/settings.xml" -Dmaven.repo.local="$HOME/.m2/repository" verify
```

Use `gradle` if the project has no wrapper, and `./mvnw` if it supplies one.
Maven is not installed by the example JVM/infrastructure image. Encrypted Maven
passwords may require an additional security-settings mount and explicit security
file configuration for your Maven version. The `-s` example does not redirect
every `${user.home}`-based file.
These mounts expose host credentials to workspace agents; prefer dedicated,
least-privilege profiles. Read-only AWS profiles can use existing tokens, but
login and refresh may fail when their cache cannot be updated.

### Sessions, removal, and recovery

- Each new task or direct-chat session saves its selected logical mounts in its durable
  platform session configuration, separately from the worker payload.
- Existing sessions retain those paths and access modes, including container
  replacement and follow-up messages. Pre-feature sessions continue with no added
  mounts. Settings changes apply only to explicitly new sessions.
- Replacement containers validate the saved mounts. Missing sources or inaccessible
  paths fail visibly, preserving the saved session and working state; Orkest never
  substitutes a path or starts a replacement conversation. Restore access to the
  original path or explicitly start a new session with compatible settings.
- Removing a workspace setting does not unmount an active container or change
  saved sessions. Stop affected runs explicitly to revoke their access; then start
  new sessions with the updated selections.
- Host files are not copied into images, encrypted, or backed up by Orkest.
  Keep paths, contents, permissions, and Docker sharing compatible across restores.
  Removing a setting or cleaning up a container never deletes the host source.

Migration 0040 stores workspace selections. Deploy matching platform binaries;
older platform versions do not enforce saved mount semantics. See
[upgrade/rollback guidance](../../operations/migrations.md#workspace-host-volumes).

## Restart and verification

- Session containers: `unless-stopped`; legacy non-session runs: no automatic restart.
  Restore follows [session rules](../../guides/sessions.md); platform shutdown leaves workers running.
- Failed starts are removed after Docker confirms cleanup. Cleanup failure retains
  container ID in launch failure for reconciliation.
- `make test-docker`: reachable Engine; named contexts need exported
  [socket](../../operations/configuration.md#docker-networking-and-paths).
  Set `ORKEST_DOCKER_TEST_MODEL_PROVIDER` and `ORKEST_DOCKER_TEST_MODEL_ID` to an
  available model before running it. Storage tests confirm selection but make no
  model requests. Fake-client unit tests need no Docker. Full prerequisites:
  [testing](../../development/testing.md).
- Host-volume access/replacement coverage without model credentials:
  `go test -tags=docker ./internal/adapters/executors/docker -run '^TestDockerHostMountsReadWriteAndReplacement$' -count=1`.
  This verifies real bind mounts, not credentialed ACP/model continuation.
  Set `ORKEST_DOCKER_TOOLCHAIN_TEST_IMAGE` to a built JVM/infrastructure image
  containing agentctl to additionally check real AWS profile discovery and Gradle
  property discovery with `--gradle-user-home`, using synthetic, non-secret files.
  Tool discovery is explicitly skipped when that test-only image is not supplied.

## Custom toolchains

[Execution profiles](../../guides/execution-profiles.md) publish composed runtime/toolchain
digests for Docker validation/execution. Active runs retain images; later runs
may adopt compatible updates. Deployment, networking, trust, retention:
[builder operations](../../operations/image-builds.md).
