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

[Terraform example](../../build/docker/examples/terraform.Dockerfile) shows a complete
toolchain image. It uses a local development base by default; replace that `FROM`
reference with the published base image digest before using it for an execution
profile. Build it locally with the base image available:

```sh
docker build \
  -f build/docker/examples/terraform.Dockerfile \
  -t orkest/toolchain-terraform:dev .
```

For execution profiles, use a published base digest, push the toolchain to your
configured registry, and supply its published reference as the toolchain image.

Python is available as both `python` and `python3` on the standard runtime `PATH`.
Its version follows the base image's Debian distribution (currently Python 3.11).
Use a virtual environment rather than modifying Debian's externally managed
system Python:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
# Or use uv with an existing project and committed uv.lock:
uv sync --frozen
```

For host files in Orkest-managed Docker runs, workspace admins configure **Docker
host volumes** directly in workspace settings; no environment variable is needed.
Use targets `~/.aws`, `~/.m2/settings.xml`, and `~/.gradle/gradle.properties` for
credential mounts into the private session home. Maven/Gradle may need explicit
settings/user-home flags because JVM `user.home` is not necessarily `HOME`.
See [credential mounts](../reference/executors/docker.md#credential-mounts) for
UI examples and commands that require no environment variables.

### AWS profiles and login

Orkest runtime processes use a restricted environment and a per-session `HOME`.
Tools installed in the toolchain image are available on the standard runtime `PATH`;
they do not require inheriting the image's `PATH`. If the image includes a JDK and
the project explicitly requires `JAVA_HOME`, add the ordinary workspace environment value
`JAVA_HOME=/opt/java/openjdk`. Apply this to new sessions; existing sessions retain
their compatible environment snapshot. Rebuild the execution profile after
publishing the updated toolchain; existing containers do not update in place.

The image contains no AWS profiles, credentials, or login tokens unless you add them
at runtime. In
Orkest-managed runtime shells, `aws configure` creates profiles under the
runtime's `$HOME/.aws`, not `/root/.aws`; do not share profiles or tokens across
sessions or workspaces.

Inside the running container, configure a named IAM Identity Center profile and
log in using a browser on your host:

```sh
aws configure sso --profile development --use-device-code --no-browser
aws sso login --profile development --use-device-code --no-browser
aws sts get-caller-identity --profile development
```

Open the displayed verification URL in your host browser and enter the displayed
code. Repeat configuration with another profile name to add more profiles;
`aws configure list-profiles` lists them. For access-key profiles, use
`aws configure --profile development` instead; these do not use SSO login.

Without persistent storage, container replacement loses these files and tokens.
Do not bake populated AWS files into images or share a credential volume between
workspaces. For Orkest-managed runs, use the workspace host-volume UI with target
`~/.aws`, or configure AWS inside the durable private session home.

Composition uses the toolchain filesystem, copies trusted `agentctl`, and installs
the pinned runtime; no arbitrary filesystem merge. Pull/pin both source images by
digest; validate runtime/writable directories on the composed image.
See [build mechanics](../development/image-builds.md).
