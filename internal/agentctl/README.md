# Agentctl package map

Paths below are relative to the repository root.

| Package | Responsibility |
| --- | --- |
| `internal/agentctl/config` | Resolved run/runtime settings, validation, cloning |
| `internal/agentctl/interaction` | Commands, approvals, updates, durable activity records |
| `internal/agentctl/run` | Run controller, private conversation engine, queue, journal and checkpoint recovery |
| `internal/agentctl/runtime` | Normalized host/session, profile and protocol contracts |
| `internal/adapters/runtime/runtimehost` | Subprocess ownership, provisioning, native storage and replay evidence |
| `internal/adapters/runtime/acp` | ACP SDK translation and model confirmation |
| `internal/adapters/runtime/profiles` | Pure native runtime configuration rendering |
| `internal/adapters/gitprovider` | Repository identity and scoped Git/CLI credential preparation |
| `internal/adapters/http/agentctl` | REST/WebSocket API and explicit wire mapping |
| `internal/contracts/agentctl/v1` | Shared wire values, without worker policy |
| `cmd/agentctl` | Composition, HTTP lifecycle and restoration supervision |

Start at `cmd/agentctl/main.go`, then `wiring.go`, `daemon.go` and
`internal/agentctl/run/controller.go`. Configuration/interaction definitions do
not depend on execution services. Concrete runtime implementations consume worker
contracts, never the run controller or server domains. The controller and private
conversation engine stay together to keep lifecycle and locking rules private.

See [ownership, compatibility and verification](../../docs/development/agentctl.md).
