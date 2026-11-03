# Execution profiles

- Workspace profiles pair runtime/toolchain images and may be shared by agents.
- Agent/runtime must match; model/provider remain on the agent.
- Without a profile, agents use deployment-default runtime images.

## Prerequisites

- Operator-enabled [image builds](../operations/image-builds.md) and registry access.
- Published registry references; no local-only aliases.
- Trusted builder: source images execute code. See [trust boundary](../operations/image-builds.md#cleanup-and-trust-boundary).

## Use profiles

| Action | Behavior |
| --- | --- |
| Create/update | Workspace Admins/inherited organization Owners/Admins; Execution profiles page |
| Save | Background build, HTTP 202; generation conflicts → 409 |
| Change runtime | Create a new profile |
| Failed build | Sanitized history/error; save explicitly to retry; last ready image stays usable |
| Select | Members select same-workspace profiles; Viewers cannot mutate/build |
| Start | Requires a ready image; new user-started run records latest ready digest |

- Builds never start/interrupt tasks. Active containers/queued prompts retain their image.
- Later runs may adopt a new image after queue drain/container cleanup, subject to
  compatible session restore.
- Changing an agent's profile affects new sessions; updating a profile affects later
  runs already bound to it.

## Prepare a toolchain image

1. Derive from supported `orkest/agentctl-base`, pinned by digest.
2. Preserve compatible Node/npm, shell, certificates, and executable permissions.
3. Pin project tools and verify downloads.

```dockerfile
FROM <published-agentctl-base>@sha256:<digest>
USER root
# Install pinned project tools and verify their downloads.
```

[Terraform example](../../build/docker/examples/terraform.Dockerfile). Official runtime
builds use `AGENTCTL_BASE_IMAGE` set to the published base digest.

Composition uses the toolchain filesystem, copies trusted `agentctl`, and installs
the pinned runtime; no arbitrary filesystem merge. Pull/pin both source images by
digest; validate runtime/writable directories on the composed image.
See [build mechanics](../development/image-builds.md).
