# Execution profiles

An execution profile pairs an agent runtime with a toolchain image. Profiles
belong to a workspace and can be shared by its agents. The agent's runtime must
match the profile; model and provider settings remain on the agent. Agents
without a profile continue to use the deployment's default runtime image.

## Configure image builds

Set `ORKEST_EXECUTION_IMAGE_REPOSITORY` to a registry repository without a tag,
such as `registry.example.com/team/orkest-execution`. Leaving it unset disables
profile builds. Source and output images must be published registry references;
local-only Docker aliases are not supported. Orkest resolves input tags to
immutable digests before each build.

Select the deployment-wide backend with `ORKEST_IMAGE_BUILDER`:

- `buildkit` (default) uses pinned `buildctl` v0.34.1 and a compatible BuildKit
  daemon. Set `ORKEST_BUILDCTL_BINARY` to override the executable path,
  `ORKEST_BUILDKIT_ADDR` to select the daemon, and `ORKEST_BUILDKIT_TLS_CA`,
  `ORKEST_BUILDKIT_TLS_CERT`, and `ORKEST_BUILDKIT_TLS_KEY` for optional mTLS.
- `docker` uses the deployment's Docker CLI and daemon. It does not need
  `buildctl` or a separate BuildKit daemon.

Both modes use Docker to resolve and validate images. `DOCKER_CONFIG` supplies
registry credentials to Docker and BuildKit; Orkest does not create registry
accounts or refresh ECR credentials. When the BuildKit and Docker daemons are
separate, both must reach the registries. The BuildKit daemon also needs package
network access. Use a Unix socket or protected connection for BuildKit; remote
TCP connections should use mTLS. `DOCKER_HOST` selects the Docker daemon used
for validation and execution.

## Prepare a toolchain image

Build the toolchain from the supported `orkest/agentctl-base` image, pinning its
digest. Keep Node/npm, a shell, certificates, and executable permissions
compatible with the selected runtime. Install project tools at pinned versions
and verify downloaded artifacts. For example:

```dockerfile
FROM <published-agentctl-base>@sha256:<digest>
USER root
# Install pinned project tools and verify their downloads.
```

See [`build/docker/examples/terraform.Dockerfile`](../build/docker/examples/terraform.Dockerfile)
for a complete example. Set `ORKEST_AGENTCTL_BASE_IMAGE` to the published base
image digest when building the official runtime images.

Orkest composes the final image from the selected toolchain image, copies in
`agentctl` from the trusted runtime image, and installs the pinned selected
runtime. It does not merge arbitrary image filesystems. Both source images are
pulled and pinned by registry digest; runtime and writable-directory checks run
on the composed image, not on each source image separately.

## Use profiles

Workspace Admins and inherited organization Owners/Admins can create and update
profiles in the Execution profiles page. Saving queues a background build and
returns HTTP 202. Saves use a generation number to reject conflicting edits
(HTTP 409); changing a profile's runtime requires creating a new profile.
Build history shows status and sanitized errors. Saving again explicitly
rebuilds after failure; builds do not retry automatically.

Members can select a same-workspace profile on an agent. Viewers cannot change
profiles, start builds, or mutate agents. A profile without a ready image cannot
start work. If a rebuild fails, the last successful image remains available.

Each new user-started run selects the profile's latest ready image and records
its digest. Active containers and queued prompts keep their current image.
Profile builds do not interrupt or start task work. Later runs of an existing
session can use the new image after the active queue drains and the container is
cleaned up, subject to runtime restoration compatibility. Session loading alone
does not prove model-memory continuation. Changing an agent's profile affects
new sessions; updating a profile affects later runs bound to it.

## Build and recovery behavior

Build jobs are stored with their profile changes in one PostgreSQL transaction.
Workers claim jobs with `SKIP LOCKED`, renew 90-second leases every 20 seconds,
and apply a 30-minute operation timeout. Multiple platform processes can build
different profiles concurrently.

BuildKit builds and pushes the image, then Orkest pulls the exported digest into
Docker and validates the runtime version and writable directories. Docker mode
validates locally, pushes, then verifies the published digest. A build becomes
ready only after validation succeeds. Promotion is conditional on the profile
generation, so an older build cannot replace a newer request. Raw build output
is discarded; persisted errors are sanitized.

After a lease expires, the worker checks for the attempt's published image and
validates its identity instead of restarting an uncertain build. Unresolved
outcomes become interrupted and require an explicit retry. Platform shutdown
joins its build worker without stopping active agent containers. A disconnected
client does not prove daemon work has stopped. Operators must clean up BuildKit
cache and Docker validation images; automatic pruning is not provided.

This is a trusted self-hosted build boundary: source images may run `ONBUILD`
hooks and installer code. Build contexts contain no task checkouts or workspace
secrets, but builds have network access. Validation runs without network access,
with resource limits, temporary mounts, and no Docker socket. Strong isolation
for hostile tenants requires a separate builder environment.

## Verification and migrations

Run the normal checks with `make check` and PostgreSQL checks with
`make test-postgres`. The image-composition test requires Docker, published
foundation images, `registry:3`, and `moby/buildkit:v0.34.1`:

```sh
ORKEST_BUILDCTL_BINARY=/path/to/buildctl make test-image-composition
```

It exercises both build backends, publishes and validates a composed image, and
checks digest recovery. To run only the Docker backend without BuildKit:

```sh
go test -tags=docker ./internal/imagebuild/docker -run '^TestDockerImageComposition/docker$' -count=1 -v
```

Migration 0025 adds profiles and run image metadata. Rolling it back removes
profiles, build history, agent selections, and run image metadata, but does not
remove images or task files. Stop profile execution and export needed
configuration before rollback. Migration 0026 restores the GitLab coding
provider constraint after the duplicate version 0023 was resolved.

For a local registry, use a full repository such as
`localhost:5001/orkest/execution-images`. Check that the registry responds at
`/v2/` before configuring builds. The default local registry port is 5001.
