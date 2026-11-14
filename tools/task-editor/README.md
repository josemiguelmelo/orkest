# Task editor Docker/browser feasibility

PR 1 of [the delivery plan](../../docs/development/task-editor-first-delivery.md).
This isolated harness creates two existing Git test checkouts, then opens them in
code-server through a minimal authenticated HTTP/WebSocket proxy. It never calls
the platform, launches agents, reads credentials or publishes images. No production
configuration, routes, frontend client or runtime pins change.

## Run

Prerequisites: Docker Engine, Git, the Node version required by `web/package.json`,
`npm --prefix web ci`, and Playwright Chromium (`cd web && npx playwright install chromium`).
The Docker daemon must see the checkout's `tmp/` directory. Run as its non-root owner.

```sh
make test-task-editor
# Or reuse a built image:
node tools/task-editor/spike.mjs
# Optional installed browser override:
TASK_EDITOR_BROWSER=/absolute/path/to/chromium make test-task-editor
```

`DOCKER` selects the CLI executable; normal Docker context/`DOCKER_HOST` settings
apply. `TASK_EDITOR_IMAGE` selects a locally built image. Each invocation creates
fresh checkouts, never clones or mounts your project repositories, and retains
screenshots, process snapshots, JSON results and test files in ignored
`tmp/task-editor/run-*/`. Containers use concrete IDs and are stopped/removed in
`finally`; interruption by SIGKILL can leave a container. Inspect containers labeled
`dev.orkest.task-editor=feasibility` before removing such leftovers. Test data remains
for inspection. Missing Docker/browser support fails the command, never skips it.

## Image and restrictions

Both image targets pin code-server **4.145.2 / Code 1.145.2**, with architecture-specific
SHA-256 values taken from the upstream GitHub release asset digests:
[release](https://github.com/coder/code-server/releases/tag/v4.145.2),
[metadata](https://api.github.com/repos/coder/code-server/releases/tags/v4.145.2).
Debian Bookworm uses a multi-platform digest. Debian's signed package repositories
provide version-pinned Git, Bash, certificates, curl and process inspection.
Transitive Debian packages can still change between image rebuilds; retired
package versions fail the build instead of silently substituting direct tooling. The bundled Node belongs to the code-server release.
Only arm64 is qualified by the recorded run; amd64's checksum is pinned but its
browser/runtime behavior needs a real run.

The harness uses the invoking owner's numeric UID/GID, read-only container root,
no capabilities, no privilege escalation, resource limits, temporary writable
home/cache and only three bind mounts: two checkout roots including `.git`, and
a read-only generated multi-root workspace. No agent state/runtime homes, host
credentials, environment secrets or Docker socket enter the editor. Production
must resolve UID/GID against daemon-visible task file ownership; do not recursively
chown existing worktrees. The default image UID/GID is 1000:1000; the harness
overrides it to demonstrate non-root file access. On this Colima VM, bind-root
directories appeared as root-owned although their files retained UID 501. The
harness gives its disposable fixture roots mode 0777 to permit file creation; it
never changes permissions on real task checkouts. This is test setup, not the
production permission policy. Production must check actual directory create and
Git access as well as UID equality before reporting read/write availability.

Repository write protection comes from Docker read-only bind mounts. Editor
settings alone cannot enforce it. Read-only mode also uses an immutable extensions
directory and disables user extensions. Automatic tasks default to off in both
modes. The pinned server has no documented terminal-disable flag: `restrict.cjs`
is a **spike-only Node PTY guard** replacing the immutable `node-pty` entry point
in the `readonly` image target (`${TASK_EDITOR_IMAGE}-ro`). The normal `editor`
target retains the original PTY implementation. A preload-only guard was tried
and failed real browser qualification because child processes bypassed it.
The replacement rejects terminal creation and is verified by the browser; it is an
upstream implementation dependency, not a complete sandbox or a supported public
code-server API. Requalify it whenever code-server/Node changes. Before production,
retain this explicitly tested guard or replace it with an equivalent verified
upstream control; never silently fall back to terminal settings alone. Trusted
mutation-authorized users remain the intended audience; Viewer isolation is deferred.

## Browser/session topology

The mock platform uses a SameSite=Lax login cookie so top-level navigation back
from the other test site can authenticate; the editor cookie remains Strict.
The mock platform listens on `http://127.0.0.1:<random>`; the editor gateway listens
on `http://localhost:<random>`. Both bind loopback. Clicking Open opens a separate
tab via a single-use, ten-second grant. A same-origin landing document establishes
an HttpOnly, SameSite=Strict editor cookie, then loads the stable launch page.
A direct cross-site 303 redirect does not reliably send that strict cookie;
this is why the landing document is required. Editor sessions last five minutes
and bind to the current in-memory generation. Grants, sessions and open sockets
are revoked before container stop; the launch page removes its old iframe, then
returns through the mock platform to obtain a fresh grant and replace its iframe
automatically. Saved files survive; unsaved buffers were lost in the observed run.

The proxy strips cookies and authorization before forwarding editor HTTP/WebSocket
traffic, verifies WebSocket Origin and resolves the upstream itself. The underlying
code-server port is loopback-only and has no independent auth: local processes can
reach it. This is an isolated single-user development topology, **not a production
gateway**. Production requires separate HTTPS hostnames, Secure cookies, real task
mutation/owner checks, expiry/revocation and a private gateway-to-container network
(PR 6). No test grants/cookies are written to evidence or ordinary logs.

See [recorded evidence](../../docs/development/task-editor-pr1-evidence.md) for exact
commands, observations, qualification limits and the exit decision.
