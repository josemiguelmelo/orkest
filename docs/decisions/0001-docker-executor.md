# Docker executor foundation

Status: Accepted for the initial executor implementation. Broader execution,
network policy, resource limits and agentctl transport remain open.

`internal/executors.Executor` is the provider-independent lifecycle contract.
`DockerExecutor` is its Go implementation using the pinned Moby Docker Engine
SDK. Create and Start are separate: orchestration must persist the environment
reference before starting it. This package does not expose an HTTP API or start
tasks automatically.

The caller constructs a Docker SDK client with `client.New(client.FromEnv)` and
passes it to `executors.NewDockerExecutor`. The caller owns client closure;
closing it must not stop containers. Images must be preloaded on that Docker
host; this implementation does not pull images or discover registry credentials.

Create accepts a Spec containing authorized organization, workspace, task and
run identities, image, command and the required environment variables. Persist
its returned Environment even on an error when the ID is present: this stable
container name permits inspection after an uncertain create response. Never
interpret an error or disconnection as proof that execution is absent. No
automatic retry, replacement, force removal or container restart is configured.
Start, Inspect, Stop and Cleanup verify all four ownership labels. These checks
are defense in depth; application membership authorization remains the caller's
responsibility. Docker host administrators remain trusted.

Each run gets a deterministic container name. A named volume scoped to
organization/workspace/task mounts at `/workspace`, retained across container
cleanup and review runs. Callers must reserve execution ownership before using
this shared task storage. Configure runtime sessions, journals and repository
working trees under that path. Cleanup removes only the container and refuses
to force removal; storage deletion requires a future explicit retention policy.
The host's default Docker bridge is used, without published ports, host mounts,
privileged mode or Docker socket access. This is container/filesystem separation,
not an implemented network isolation policy.

Inspect exposes state, exit code and executor-network addresses. Addresses do
not establish connectivity or authenticate agentctl. Platform-to-agentctl
transport and durable OpenCode continuation are not implemented or verified by
this foundation. SDK errors are internal operational errors and must not be
returned verbatim to browser clients or ordinary logs, since a daemon may echo
submitted configuration.

Verification: `go test ./internal/executors` exercises the lifecycle through an
injected Docker client. `make check` runs repository checks. These tests do not
verify a real daemon or the requirements' recovery acceptance scenarios.
