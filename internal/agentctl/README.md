# Agentctl package map

| Package | Responsibility |
| --- | --- |
| `config` | Resolved run/runtime settings, validation, cloning |
| `interaction` | Commands, approvals, updates, durable activity records |
| `run` | Run controller, private conversation engine, queue, journal, checkpoint recovery |
| `runtime` | Normalized host/session, profile, and protocol contracts |
| `runtimehost` | Subprocess ownership, provisioning, native session storage, replay evidence |
| `runtime/acp` | ACP SDK translation and callbacks |
| `runtime/opencode` | Pure OpenCode native configuration rendering |
| `httpapi` | One REST/WebSocket server and wire representations |
| `daemon` | Server lifecycle and restoration supervision |

Start at `cmd/agentctl/main.go`, then `run/controller.go`.
Configuration and interaction definitions do not depend on execution services.
Runtime implementations depend on contracts, never the run controller. The
controller and private conversation engine stay together to keep lifecycle and
locking rules private. There is no forwarding package at the root.

See [architecture and verification](../../docs/runtime-providers.md).
