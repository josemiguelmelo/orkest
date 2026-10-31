# Docker agentctl executor

The Docker executor starts one `agentctl` container per run. It targets the local
Docker Engine configured with `DOCKER_HOST` (or the Engine's default socket). The
executor binds a dynamically allocated host port on `127.0.0.1` by default; the
container listens on port 8081. When `ORKEST_AGENTCTL_NETWORK` is set, it instead
joins each agentctl container to that Docker network and uses the container name
and port 8081 as its endpoint. Compose sets this to the shared `orkest` network
so Orkest can reach its agentctl containers directly. It returns the Docker
container ID and endpoint to the launch service.

Build the images from the repository root:

```sh
make docker-agentctl-base
make docker-agentctl-opencode
make docker-agentctl-claude-code
make docker-agentctl-codex
```

For the local Compose registry, build and push all runtime images with:

```sh
make docker-agentctl-push-local
```

This starts the `registry` Compose service, pushes images to
`localhost:${REGISTRY_PORT:-5001}/orkest`, and tags matching local images under
`registry:5000/orkest`, which is the default reference used by the Compose Orkest
service. Override `REGISTRY_PORT`, `LOCAL_REGISTRY`, `CONTAINER_REGISTRY`, or
`AGENTCTL_REGISTRY_NAMESPACE` when using a different local registry setup.

To build without Make, run from the repository root:

```sh
docker build -f build/docker/agentctl-base.Dockerfile -t orkest/agentctl-base:dev .
docker build -f build/docker/agentctl-opencode.Dockerfile -t orkest/agentctl-opencode:local .
docker build -f build/docker/agentctl-claude-code.Dockerfile -t orkest/agentctl-claude-code:local .
docker build -f build/docker/agentctl-codex.Dockerfile -t orkest/agentctl-codex:local .
```

The runtime Dockerfiles read `internal/core/runtime/versions.json` from the build
context, so no runtime version build arguments are needed. The tag is your choice;
configure `AGENTCTL_OPENCODE_IMAGE`, `AGENTCTL_CLAUDE_CODE_IMAGE`, or
`AGENTCTL_CODEX_IMAGE` when using a custom tag. `AGENTCTL_BASE_IMAGE` remains an
optional build argument for a custom base image.

The base image contains the `agentctl` binary, Git, GitHub CLI 2.103.2, and
GitLab CLI 1.125.0. It does not include BuildKit; image composition runs on the
platform host. Each runtime image extends that base and installs only its pinned runtime:
OpenCode 2.1.4 in `orkest/agentctl-opencode:2.1.4`, or the Claude Code ACP
adapter 0.99.0 in `orkest/agentctl-claude-code:0.99.0`, or Codex ACP adapter
2.3.0 in `orkest/agentctl-codex:2.3.0`. The `docker-agentctl-images` target
builds the base and all runtime images. Override
`AGENTCTL_BASE_IMAGE`, `AGENTCTL_OPENCODE_IMAGE`, or
`AGENTCTL_CLAUDE_CODE_IMAGE`, or `AGENTCTL_CODEX_IMAGE` to use different tags.
The executor chooses the image matching the run's runtime; these environment
variables configure those per-runtime images. The executor passes the assigned runtime to agentctl at
startup, and agentctl rejects run configuration for any other runtime. Container
names include the runtime and a hash of the run ID for identification and recovery.

For each organization/workspace/task scope, the executor creates durable
`work` and `agentctl-state` directories beneath `ORKEST_DATA_DIR/tasks/<scope-hash>`
and mounts them at `/workspace` and `/agentctl-state`. Every repository selected for
the task is checked out under `/workspace/repositories/<provider>/<repository-id>`
on its default branch for a new checkout. Existing checkouts retain their current
branch and working tree. The agent creates a task-specific branch only when the
requested work needs changes. Those checkouts persist across review iterations. The hash keeps identifiers
out of host paths and container labels. The directories are mode 0700 and the
container runs as the platform process UID/GID so files remain writable by the
platform user. The agentctl state directory must remain on persistent storage.

The GitHub coding provider resolves each integration PAT on the platform. It uses a
temporary credential helper for checkout, then sends the PAT to agentctl in the
encrypted task-session configuration. Git receives URL-scoped HTTP authorization
through process configuration. Agentctl also adds a `gh` wrapper to the runtime
`PATH`; when run inside a registered checkout, it selects that repository's PAT
through `GH_TOKEN` without writing credentials to gh's auth file. Run `gh pr create`
from the repository checkout. Public repository registrations without an
integration continue to use the legacy checkout adapter and have no authenticated
push or `gh` access.

The container does not restart automatically. Agentctl session restoration is
not implemented, so automatically restarting it after a daemon restart could
leave a healthy but unconfigured process and misrepresent task recovery. Normal
platform shutdown leaves the container running. Failed container starts are
removed when Docker confirms the cleanup operation; if cleanup itself fails, the
container ID is retained in the launch failure record for reconciliation.

Run the Docker smoke test with a reachable local Engine and the built image. The Moby client reads `DOCKER_HOST`; when Docker CLI is using a named context, export that context's socket first:

```sh
export DOCKER_HOST="$(docker context inspect --format '{{(index .Endpoints "docker").Host}}')"
make test-docker
```

Unit tests use a fake Docker Engine client and do not require Docker access.

## Custom toolchains

Workspace execution profiles can asynchronously compose toolchain images with
one pinned runtime and publish them using deployment-managed Docker credentials.
Set `ORKEST_EXECUTION_IMAGE_REPOSITORY` to enable builds and
`ORKEST_IMAGE_BUILDER=buildkit|docker` to select composition. BuildKit mode
(default) uses `ORKEST_BUILDKIT_ADDR`; Docker mode builds/pushes through Docker
without needing buildctl. Docker pulls, validates, and executes published digests
in both modes. Both
daemons must reach the input/output registries. See [execution profiles](../execution-profiles.md)
for configuration, supported foundations, profile permissions, and verification.
Active runs retain their image; later user-started runs can adopt an update while
restoring the same session. Registry and local image retention are operator-managed.
