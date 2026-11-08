# Kubernetes agentctl executor

`ORKEST_EXECUTOR=kubernetes` launches one directly managed Pod and one internal
ClusterIP Service per run. Docker remains the default; Local remains available
for trusted single-user development. No Helm chart is included.

## Requirements

- Run the Orkest platform inside the target Kubernetes cluster. The initial
  implementation uses the platform Pod's service-account token and CA bundle.
  Kubeconfig-based remote clusters are not supported.
- Set `ORKEST_KUBERNETES_NAMESPACE`, `ORKEST_KUBERNETES_INSTALLATION_ID`, and
  `ORKEST_KUBERNETES_DATA_PVC`. The PVC must already exist and provide durable,
  shared ReadWriteMany storage. Mount that same PVC in the platform at
  `ORKEST_DATA_DIR/agent-executions`; optionally set
  `ORKEST_KUBERNETES_DATA_SUBPATH` to the PVC-relative prefix used by both.
- Set platform and worker UID/GID to the same non-root values with write access
  to the PVC. `ORKEST_KUBERNETES_UID` and `ORKEST_KUBERNETES_GID` default to the
  platform process IDs. Task directories are mode 0700.
- Set `ORKEST_AGENT_MCP_URL` to an HTTP endpoint reachable from worker Pods.
  The loopback fallback is rejected for Kubernetes execution.
- Give the platform service account only namespaced Pod, Service, Secret
  lifecycle and Pod-log permissions. Worker Pods disable service-account token
  automount and have no Kubernetes API permissions. Keep worker Services out of
  ingress and public load balancers; allow platform-to-worker TCP 8081 and
  worker-to-platform MCP traffic in cluster network policy.
- Configure runtime images with the existing `AGENTCTL_*_IMAGE` variables. The
  platform's image pull credentials must be available to the namespace. Runtime
  image overrides and execution-image digests are selected per run.

Provider environment values named by `ORKEST_AGENT_PROVIDER_ENV` are copied into
a run-scoped Kubernetes Secret and exposed to that Pod by Secret key reference.
The Secret is removed after confirmed Pod termination. `ORKEST_CODEX_AUTH_FILE`
host-path mounts are not supported by this executor; Codex API-key environment
configuration continues to work.

## Lifecycle and storage

The executor prepares selected repositories on the shared PVC using the existing
checkout providers. A worker receives only that task's `work` and `agentctl-state`
subpaths at `/workspace` and `/agentctl-state`. Direct conversations keep the
same storage scope as other executors. Pods use `restartPolicy: Never`; platform
shutdown does not delete a worker. A stable Service DNS name is persisted as the
worker endpoint.

Cancellation requests graceful Pod deletion with a UID precondition, waits for
API-confirmed disappearance, then removes the owned Service and run Secret. PVC
contents are retained. An API error or timeout is reported as unresolved
ownership; it does not authorize starting a replacement. If a node is unreachable,
an operator must fence it before resuming work against the same files.

## Current qualification limits

This executor has not yet been qualified against a real cluster, shared-volume
provisioner, all five runtimes, platform restart, or live session continuation.
The current execution-profile image builder still uses the Docker image store;
profile build and validation in a Docker-free Kubernetes installation are not
supported yet. Codex host-login Secret mapping, configurable resource requests
and limits, image-pull Secret references, Pod log streaming, kubeconfig mode,
network-policy manifests, and end-to-end failure/recovery qualification remain
follow-up work. The executor therefore does not yet claim full Kubernetes feature
parity.

The platform service account must be provisioned by the operator. This executor
does not create RBAC, namespaces, PVCs, network policies, or Helm resources.
