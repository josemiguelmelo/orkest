# Task editor PR 1 feasibility evidence

Date: 2026-11-14. Result: **passed on real Docker and Chromium, linux/arm64**.
This qualifies the isolated spike in [the delivery plan](task-editor-first-delivery.md),
not production task editing or the later execution handoff.

## Reproduce

See [harness instructions](../../tools/task-editor/README.md) for prerequisites.
The successful command in this workspace was:

```sh
TASK_EDITOR_BROWSER=/private/tmp/orkest-playwright/chromium_headless_shell-1248/chrome-headless-shell-mac-arm64/chrome-headless-shell make test-task-editor
```

Without an installed-browser override, `make test-task-editor` uses Playwright's
Chromium. The target builds both local image targets, verifies the downloaded
release archive and runs the browser harness. It does not publish either image.

Successful local evidence is retained in `tmp/task-editor/run-DvnpTv/`:
`evidence.json`, `rw.png`, `ro.png`, two Git checkouts, the workspace manifest,
saved/untracked files and the stopped terminal's heartbeat. These ignored artifacts
are machine-local; rerunning generates a fresh directory. Both screenshots were
visually inspected. No grants, browser cookies or real credentials are recorded.

| Component | Observed version / pin |
| --- | --- |
| Host / container architecture | macOS arm64 / Colima linux/arm64 |
| Docker client / Engine | 29.9.2 / 29.5.2, API 1.54 |
| Node / Playwright | 26.13.0 / repository-pinned 1.64.1 |
| Headless Chromium | 156.0.8078.4 |
| code-server / Code | 4.145.2 / 1.145.2, commit `302bc77436721f5a474e06d1fe34cf8f86195d5e` |
| Debian base index digest | `sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc` |
| Git | `1:2.39.5-0+deb12u3` |
| Bash | `5.2.15-2+b13` |
| curl | `7.88.1-10+deb12u15` |
| Certificates | `20250419~deb12u2` |
| procps | `2:4.0.2-3` |
| Container UID:GID | `501:20`, non-root |

The [Dockerfile](../../build/docker/task-editor.Dockerfile) pins those direct
packages, the base digest, code-server version and both architecture checksums.
The archive's SHA-256 verification passed during the real arm64 build. Checksums
come from the [upstream release metadata](https://api.github.com/repos/coder/code-server/releases/tags/v4.145.2).
The successful local read/write image identifier was
`sha256:b74b7822e4ee0d0ec936409c99af015ee1a0d385f50477deb07ec83b91e9b992`;
read-only was `sha256:d4c204f9c426f0d3c8165cc5925ef05fd0411d4ecafb259d45245c5073b353d1`.
Transitive Debian dependencies and local build attestations can change on rebuild;
these local identifiers are evidence, not published distribution references.

## Observed acceptance

| Check | Actual result |
| --- | --- |
| Existing multi-root checkouts | Browser explorer displayed alpha and beta; existing task branches retained |
| Browser editing | Changed alpha's README and saved it; host-side assertion read the exact saved bytes |
| Git inspection / integrated terminal | Browser terminal executed `git status`; reported the README modification and created an untracked file in beta |
| Read-only file and Git metadata | Writes to each root's README and `.git/config` failed with `Read-only file system` |
| Terminal denial | Real browser displayed the explicit read-only PTY rejection; process snapshot contained no shell executable |
| Extension installation | The valid offline fixture VSIX installed in read/write mode, failed installation into the immutable read-only extension directory, and remained absent there |
| Automatic workspace tasks | Both checkouts contained a folder-open task; its sentinel remained absent with automatic tasks disabled |
| Complete terminal shutdown | Snapshot showed the interactive shell, background Bash loop and sleep descendant; stop confirmed `State.Running=false`, removed that concrete container, and the heartbeat ceased changing |
| Saved/untracked files and branches | Saved README, beta's untracked file and both `task/editor-spike` branches survived recreation |
| Authenticated proxy | Anonymous HTTP denied; single-use grant replay and foreign-origin grant mutation denied; authenticated browser assets and WebSockets worked |
| Generation revocation / reconnect | Old cookie received 403 after stop; the existing tab automatically reauthenticated and loaded generation 2 read-only, then generation 3 read/write |
| Unsaved buffer | Deliberately unsaved `UNSAVED-SPIKE` edit disappeared on recreation; saved bytes remained intact |
| Mount isolation | Inspected exact mount allowlist: two fixture roots and manifest, plus temporary home and `/tmp`; no agent state, credentials, host home or Docker socket mounted |

Additional checks passed: `make lint` (0 issues), Node syntax checks for both
scripts, `bash -n tools/task-editor/entrypoint.sh`, Prettier checks for the Node
scripts, and `git diff --check`. No Go, worker/session, OpenAPI or UI production code
changed; model, PostgreSQL, real OpenCode continuation and full `make check` were
not run for this isolated image/harness increment.

## Exit decision and implementation consequences

**Proceed to PR 2.** Docker mount enforcement, complete terminal-environment
termination and authenticated browser recreation are feasible in this topology.
The feature remains unavailable in production; no enablement or deployment claim.

1. **Use a qualified read-only image variant.** The pinned server has no terminal
   disable CLI option. A Node preload hook failed actual qualification because
   child processes bypassed it. The `readonly` build target instead replaces the
   immutable `node-pty` entry point with `restrict.cjs`, which rejects PTY creation.
   The normal target retains upstream PTY behavior. This is a deliberate spike
   dependency on upstream internals. PR 3 must carry forward equivalent verified
   enforcement and pin both images; every upgrade requires real browser checks.
   Read-only mounts remain the repository integrity boundary. This is not a
   sandbox for untrusted Viewers.
2. **Separate editor browser session.** The mock uses distinct loopback hosts,
   single-use ten-second launch grants, five-minute generation-bound HttpOnly
   editor cookies, an authenticated HTTP/WebSocket proxy and socket revocation.
   An editor-origin landing document is required before navigating with its
   SameSite=Strict cookie; a cross-site 303 redirect failed this test. The mock
   platform cookie uses Lax for top-level return navigation. The stable launch
   page removes the old iframe before reauthentication. Production PR 6 must
   implement separate HTTPS origins, Secure cookies, real user/task authorization
   and private upstream networking; local loopback ports are reachable by other
   host processes and do not establish those production protections.
3. **Validate actual daemon-visible permissions.** Numeric file ownership worked
   at UID:GID 501:20, but this VM presented bind-root directories as root-owned.
   Only disposable fixture roots were given mode 0777 for file creation. Do not
   apply that workaround to real tasks. PR 2/3 must check actual directory and Git
   access using the intended non-root identity and report incompatible ownership.
4. **Expect lost unsaved work and terminated commands.** Saved files survive;
   browser buffers and running terminal processes did not. Preserve the planned
   user explanation and reconnect behavior. No agent or session storage changes
   are needed for this spike.

amd64, production HTTPS/DNS/cookie policy, real task authorization, worker queues,
agent handoff, restart reconciliation, Local and Kubernetes remain unqualified by
this PR. No images were published and no external resources or PRs were created.
