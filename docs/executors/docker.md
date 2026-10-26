# Docker agentctl executor

The Docker executor starts one `agentctl` container per run. It targets the local
Docker Engine configured with `DOCKER_HOST` (or the Engine's default socket). The
executor binds a dynamically allocated host port on `127.0.0.1`; the container
listens on port 8081. It returns the Docker container ID and the host endpoint to
the launch service.

Build the images from the repository root:

```sh
make docker-agentctl-base
make docker-agentctl-opencode
```

The base image contains the `agentctl` binary and Git. The runtime image extends that
base and installs pinned OpenCode 2.1.4. Override `AGENTCTL_BASE_IMAGE` or
`AGENTCTL_OPENCODE_IMAGE` when building or running a differently tagged image.
The platform-side executor reads `ORKEST_DATA_DIR` through its constructor and
uses `orkest/agentctl-opencode:2.1.4` when no image is configured.

For each organization/workspace/task scope, the executor creates durable
`work` and `agentctl-state` directories beneath `ORKEST_DATA_DIR/tasks/<scope-hash>`
and mounts them at `/workspace` and `/agentctl-state`. Every repository registered
in the task workspace is checked out under `/workspace/repositories/<provider>/<repository-id>`
on a task-specific branch before the container starts. Those checkouts persist across
review iterations. The hash keeps identifiers
out of host paths and container labels. The directories are mode 0700 and the
container runs as the platform process UID/GID so files remain writable by the
platform user. The agentctl state directory must remain on persistent storage.

No provider credentials are passed into containers in this first implementation.
Model requests requiring credentials will fail until a credential integration
is added. The GitHub repository provider currently supports public HTTPS remotes.

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
