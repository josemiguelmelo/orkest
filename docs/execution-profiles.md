# Execution profiles

A workspace can contain multiple profiles. Each profile pairs one supported
agent runtime with a toolchain image. Agents select profiles; several agents can
share one. Keep model/provider choices on the agent. Runtime and selected profile
must agree. Existing agents without a profile retain deployment defaults.

## Configuration

Set `ORKEST_EXECUTION_IMAGE_REPOSITORY` to a registry repository without a tag,
for example `registry.example.com/team/orkest-execution`. Leaving it unset disables
new builds. Choose the build/push backend with `ORKEST_IMAGE_BUILDER`:

- `buildkit` (default): standalone BuildKit through `buildctl`. Install pinned
  `buildctl` v0.34.1 and provide a compatible BuildKit daemon.
- `docker`: build and push through the deployment's Docker CLI/daemon. This mode
  does not require `buildctl` or a standalone BuildKit daemon.

Both implement the same worker contract, use the same controlled build context
and installation recipe, and publish to the configured registry. Docker resolves
registry inputs, pulls, validates, and executes artifacts in both modes. The
selection is deployment-wide; switching it does not interrupt active agent runs.
The `ORKEST_BUILDCTL_BINARY` and `ORKEST_BUILDKIT_*` settings are ignored
in Docker mode. Registry credential settings still apply to both backends.

- `ORKEST_BUILDCTL_BINARY`: executable path (default `buildctl` on PATH).
- `ORKEST_BUILDKIT_ADDR`: endpoint, such as `unix:///run/buildkit/buildkitd.sock`,
  `docker-container://buildkitd`, or a TLS-protected `tcp://builder:1234`.
  When unset, buildctl uses `BUILDKIT_HOST` or its standard default socket.
- `ORKEST_BUILDKIT_TLS_CA`, `ORKEST_BUILDKIT_TLS_CERT`, and
  `ORKEST_BUILDKIT_TLS_KEY`: optional TLS files; certificate/key must be paired.
- `DOCKER_CONFIG`: deployment-managed registry credentials/credential helpers
  consumed by buildctl and Docker CLI. Orkest does not provision registry accounts
  or refresh ECR credentials itself.

Use a Unix socket, protected container connection, or mTLS for remote BuildKit.
BuildKit's daemon needs network access to input/output registries and package
sources; configure registry TLS/HTTP/mirrors on that daemon independently of Docker.
The BuildKit and Docker daemons can be separate, but both must reach the same
registry. `DOCKER_HOST` still selects the validation/execution Docker daemon.

`AGENTCTL_OPENCODE_IMAGE` and `AGENTCTL_CLAUDE_CODE_IMAGE` select trusted agentctl
source images. For profiles these and the toolchain must be published registry
references in both modes. Docker-local aliases are not supported by the shared
composition contract.
Input tags are resolved and recorded as immutable registry digests before building.
The runtime-image defaults still apply to agents without a profile.

Build your toolchain from the supported `orkest/agentctl-base` foundation:

```dockerfile
FROM <your-published-orkest-agentctl-base>@sha256:<digest>
USER root
# Install project tools with pinned versions and verified downloads.
```

[`build/docker/examples/terraform.Dockerfile`](../build/docker/examples/terraform.Dockerfile)
is a complete sample that adds Terraform 1.12.2 with architecture-specific
SHA-256 verification. Supply the published base image digest as
`ORKEST_AGENTCTL_BASE_IMAGE` when building it.

The current foundation uses Debian Bookworm and Node 22.19.0, with agentctl, Git,
GitHub/GitLab CLIs, certificates, and curl. Your image must retain compatible
Node/npm, shell, certificates, and executable permissions. Orkest uses that
filesystem as its final base, refreshes agentctl from the selected trusted source,
and installs the selected pinned runtime through the same script used by official
runtime images. Arbitrary image filesystems are not merged.

## User workflow

Workspace Admins and inherited organization Owners/Admins create/update profiles
on the Execution profiles page. Saving queues a build. The API returns 202;
processing occurs in the platform background with one concurrent job per process.
Build history exposes bounded status results and sanitized failure messages.
Saving again explicitly retries/rebuilds. Each save includes the last observed
generation; conflicting edits return 409. Runtime cannot change on an existing
profile; create another profile instead.

Members can select a same-workspace profile on an agent. Viewers cannot create
profiles, request builds, or mutate agents. Unprepared profiles can be configured,
but cannot start work. A failed rebuild keeps the last successful image usable;
the UI distinguishes that environment from pending changes.

A session retains its selected profile. Each new user-started run resolves the
profile's current ready image; prompts queued inside an active run keep its image.
Building or updating an image never interrupts work, approval waits, or starts a
task. Once the queue drains and normal container cleanup finishes, another user
message can start a run restoring the same native session with the updated image.
Runtime restoration compatibility checks still apply; session loading does not
prove real model-memory continuation. Changing an agent's profile affects new
sessions; updating its existing profile affects subsequent runs of bound sessions.

Each run records its chosen registry digest, and session snapshots retain the
current executor image metadata separately from the agentctl HTTP payload. Active
containers are unaffected by profile edits. Registry retention must preserve
images required by retained work and recovery. Automatic pruning is not provided.

## Worker and recovery

Profiles and build jobs are saved in one transaction. Workers claim PostgreSQL
rows with `SKIP LOCKED`, renew a 90-second lease every 20 seconds, and apply a
30-minute operation timeout. Multiple platform processes may claim different jobs.
There is no external broker. Output tags are unique to build attempts.

Resolved input identities and runtime versions are stored before the build.
BuildKit builds and pushes directly using the image exporter. Orkest reads the
exported digest from BuildKit metadata, pulls that exact digest into Docker, then
probes runtime versions and writable mounts under the executor UID/GID. The image
is not promoted to ready until validation succeeds. Docker mode uses `docker build`
and `docker push`, validates locally before pushing, then verifies the published
artifact through the same image store. Validation is not an ACP/model qualification test. Output
promotion is conditional on the profile generation, so an older result cannot
replace a newer requested build. Ordinary failures do not retry automatically.
Raw BuildKit/Docker/npm output is discarded to keep credentials out of persisted diagnostics.

After an expired lease the worker inspects the pushed attempt image and validates
its build identity; it never automatically restarts that build. An unresolvable
outcome becomes interrupted. Explicit retry creates a new attempt/output tag;
operators should inspect unfinished daemon work before retrying uncertain effects.
Platform shutdown cancels build operations and joins its worker without stopping
active agent containers. A lost build client connection does not prove daemon work
terminated. Docker validation images and BuildKit cache remain until operator cleanup.

This is a trusted self-hosted build boundary. Source images can contain executable
ONBUILD hooks and installer code. Build contexts contain no task checkouts or
workspace secrets, but builds require network access. Validation has no network,
a memory/CPU/PID limit, temporary mounts, and no Docker socket. Strong isolation
for hostile tenants requires a separate builder environment and is deferred.

## Verification and operational limits

`make check` covers normal Go/frontend checks. `make test-postgres` includes queue
concurrency and migration checks. `TestExecutionProfileMigration` can run against
an isolated schema independently of unrelated historical migrations.

With built foundation images, Docker, `buildctl` v0.34.1, and the pinned
`registry:3` and `moby/buildkit:v0.34.1` images available:

```sh
ORKEST_BUILDCTL_BINARY=/path/to/buildctl make test-image-composition
```

This creates a temporary local registry and separate privileged BuildKit test daemon,
composes and publishes OpenCode through buildctl, validates with Docker,
and recovers its digest, and probes a replacement container. It does not prove
credentialed model continuation or ECR integration. Test containers are removed;
Docker validation images remain for operator cleanup; the temporary BuildKit
daemon and its cache are removed. The test exercises both backends; to verify
Docker alone without buildctl or a BuildKit daemon, run:

```sh
go test -tags=docker ./internal/executors/docker -run '^TestDockerImageComposition/docker$' -count=1 -v
```

Migration 0025 is additive. Its rollback removes profiles, build provenance,
agent selections, and run image metadata; it does not remove images or task files.
Stop profile execution and export needed configuration before rollback.

Before the BuildKit migration, verified on 2026-10-31: `make check` (including lint and 42 frontend tests),
`make test-docker` (both official runtime builds and launch/restoration tests),
the real local-registry composition test, isolated profile migration/queue tests,
and worker/orchestration race tests passed. Full `make test-postgres` is blocked
by the repository's pre-existing duplicate migration version 0023. The duplicate
files were left unchanged; deployment migrations remain blocked until resolved.

BuildKit build/push behavior follows the [upstream BuildKit v0.34.1 documentation](https://github.com/moby/buildkit/blob/v0.34.1/README.md),
including registry credentials, image-exporter digests, container connections, and
remote TLS configuration. The PostgreSQL queue, leases, input snapshots, and
session image selection are unchanged by the builder migration.

After the BuildKit migration, `make check` passed, including lint, Go tests,
frontend tests/type checks, generated-client verification, and builds. The real
standalone BuildKit v0.34.1 build/push/validation/reconciliation test passed against
a temporary local registry. No cloud-registry credentials or live model calls were
used; those integrations remain separately qualified. No database changes were
needed for this builder replacement; the existing migration collision remains.

With both backends enabled in code, `make check` passed. Real registry composition
subtests passed separately for Docker and standalone BuildKit, including publishing,
digest verification, reconciliation, and replacement-container probes. Docker mode
also has a selection test proving it does not require a buildctl executable.

The duplicate migration-number startup blocker was subsequently repaired by
keeping task repositories at 0023 and moving GitLab provider constraints to 0026.
The updated local backend serves the profile routes. Missing registry configuration
now returns `execution_images_not_configured` with a specific UI setup message.
Fresh-schema migration checks pass. The complete PostgreSQL repository suite
still exposes a separate existing task-repository RESTRICT deletion error; it
must not be reported as passed.

For a local registry, configure a full repository such as
`localhost:5001/orkest/execution-images`, not just `localhost:5001` (which image
reference syntax interprets as a tag). Check `/v2/` before configuring builds. On
macOS, a 403 response with `Server: AirTunes` indicates AirPlay Receiver occupies
the port; choose a free registry port or disable AirPlay Receiver explicitly.

The migration-number repair passes fresh-schema and isolated profile migration
checks, and `make check` passes. The running local API was replaced with the new
binary and its served contract now includes execution-profile endpoints.
