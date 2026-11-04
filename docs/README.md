# Documentation

Start with [local setup](../README.md).

| Topic | Read |
| --- | --- |
| Product requirements | [Requirements](requirements.md) |
| Domain terms | [Glossary](glossary.md) |
| Code organization | [Architecture](architecture.md), [technology stack](tech-stack.md) |
| Implementation/verification | [Status](status.md) |
| Agent configuration | [Agent configuration](guides/agent-configuration.md), [runtimes](reference/runtimes/README.md) |
| Task lifecycle | [Workflows](guides/workflows.md), [sessions](guides/sessions.md), [artifacts](guides/artifacts.md) |
| Repositories/imports | [Integrations](guides/integrations.md) |
| Platform setup | [Quick start](../README.md#run-locally), [Compose](guides/local-setup.md), [configuration](operations/configuration.md) |
| Agent execution | [Docker](reference/executors/docker.md), [Local](reference/executors/local.md), [execution profiles](guides/execution-profiles.md) |
| Standalone worker | [agentctl](guides/agentctl.md), [API reference](reference/agentctl-api.md) |
| Development/testing | [Contributing](../CONTRIBUTING.md), [testing](development/testing.md) |
| Operations/upgrades | [Storage and secrets](operations/storage-and-secrets.md), [image builds](operations/image-builds.md), [migrations](operations/migrations.md) |

## Developer references

- [Architecture and package boundaries](architecture.md)
- [Lifecycle and orchestration improvement plan/evidence](development/architecture-improvements-plan.md)
- [Verification record and remaining limits](status.md#verification-record)
- [Agentctl ownership and compatibility](development/agentctl.md)
- [Session persistence and continuation](development/sessions.md)
- [Artifact publication and input snapshots](development/artifacts.md)
- [Workflow transactions and event delivery](development/workflows.md)
- [Delivery claims, fencing, and recovery](development/workflow-delivery.md)
- [Image build adapters](development/image-builds.md)
- [Webapp architecture](development/webapp.md)

## Documentation conventions

- One authoritative home per detail; link elsewhere. Keep cautions beside actions.
- Requirements: labeled confirmed/proposed/open rules. Guides: current procedures.
  Reference: APIs. Development: mechanics. Operations: deployment/upgrade hazards.
  [Status](status.md): implementation versus verification.
- Short sentences, bullets for rules/steps, examples, comparison tables.
- Label Mermaid arrows: calls, dependencies, state changes.
- Versions: manifests; defaults: `.env.example`.
- Evidence: date, commit, environment; past results are not unconditional guarantees.
