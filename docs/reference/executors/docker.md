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
  runtime each. BuildKit runs on the platform. Pins: [inventory](../../../internal/core/runtime/versions.json).
- Push target starts registry, pushes to `localhost:${REGISTRY_PORT:-5001}/orkest`,
  tags Compose aliases under `registry:5000/orkest`.
- Registry overrides: `REGISTRY_PORT`, `LOCAL_REGISTRY`, `CONTAINER_REGISTRY`, `AGENTCTL_REGISTRY_NAMESPACE`.
- Base selector: `AGENTCTL_BASE_IMAGE`. Runtime selectors: `AGENTCTL_OPENCODE_IMAGE`,
  `AGENTCTL_CLAUDE_CODE_IMAGE`, `AGENTCTL_CODEX_IMAGE`, `AGENTCTL_GITHUB_COPILOT_IMAGE`, `AGENTCTL_KIRO_IMAGE`.
- Pass selected runtime to agentctl; reject mismatches.
- Rebuild after worker changes. Existing processes keep their binary; drain queues
  before adoption and preserve [session compatibility](../../guides/sessions.md).

## Durable files and credentials

- Scope: organization/workspace/task. Root: `ORKEST_DATA_DIR/tasks/<scope-hash>`;
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

## Restart and verification

- Session containers: `unless-stopped`; legacy non-session runs: no automatic restart.
  Restore follows [session rules](../../guides/sessions.md); platform shutdown leaves workers running.
- Failed starts are removed after Docker confirms cleanup. Cleanup failure retains
  container ID in launch failure for reconciliation.
- `make test-docker`: reachable Engine; named contexts need exported
  [socket](../../operations/configuration.md#docker-networking-and-paths).
  Fake-client unit tests need no Docker. Full prerequisites: [testing](../../development/testing.md).

## Custom toolchains

[Execution profiles](../../guides/execution-profiles.md) publish composed runtime/toolchain
digests for Docker validation/execution. Active runs retain images; later runs
may adopt compatible updates. Deployment, networking, trust, retention:
[builder operations](../../operations/image-builds.md).
