# Operate execution image builds

Deployment setup for [execution profiles](../guides/execution-profiles.md).

## Configuration

| Setting | Effect |
| --- | --- |
| `ORKEST_EXECUTION_IMAGE_REPOSITORY` | Registry repository without tag, e.g. `registry.example.com/team/orkest-execution`; unset disables builds |
| `ORKEST_EXECUTION_PROFILE_BUILD_WORKERS` | Concurrent builds per platform process; default `1`; invalid/nonpositive values use default |
| `ORKEST_IMAGE_BUILDER` | `buildkit` (default) or `docker` |
| `ORKEST_BUILDCTL_BINARY`, `ORKEST_BUILDKIT_ADDR` | buildctl executable and compatible daemon |
| `ORKEST_DOCKER_REGISTRY_HOST`, `ORKEST_BUILDKIT_REGISTRY_HOST` | Optional registry host mapping when the Docker and BuildKit daemons reach the same registry through different addresses |
| `ORKEST_BUILDKIT_TLS_CA`, `ORKEST_BUILDKIT_TLS_CERT`, `ORKEST_BUILDKIT_TLS_KEY` | Optional mTLS |
| `ORKEST_BUILDKIT_DAEMONLESS=true` | Bundled `buildctl-daemonless.sh` starts rootless BuildKit per build inside platform container |

- BuildKit is pinned to v0.34.1. Docker mode needs only Docker CLI/daemon, no buildctl/separate BuildKit.
- Daemonless uses platform-container network/filesystem, with no separate daemon or
  persistent cache. Allow required user-namespace/mount operations; see [rootless requirements](https://github.com/moby/buildkit/blob/v0.34.1/docs/rootless.md#containerized-deployment).
- Source/output images require published registry references; no local aliases.
  Resolve input tags to immutable digests before each build.
- Both modes use Docker for resolution/validation. `DOCKER_HOST` selects validation/execution daemon.
- `DOCKER_CONFIG` supplies registry credentials to both tools; Orkest neither creates
  accounts nor refreshes ECR credentials.
- Separate daemons must both reach registries; BuildKit also needs package network access.
  Use Unix/protected connections; remote TCP should use mTLS.
- Local registry example: `localhost:5001/orkest/execution-images`; check `/v2/` first.

## Cleanup and trust boundary

- Operators retain registry/local images and prune BuildKit cache/Docker validation images;
  no automatic pruning.
- Trusted self-hosted builds execute source `ONBUILD` hooks/installers with network
  access. Contexts exclude task checkouts/workspace secrets.
- Validation: no network/Docker socket; resource limits and temporary mounts.
  Hostile tenants require a separate builder environment.

See [leases/validation/recovery](../development/image-builds.md), [testing](../development/testing.md),
and [rollback](migrations.md#execution-profiles).
