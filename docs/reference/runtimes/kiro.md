# Kiro CLI runtime

## Configuration

- Runtime/provider `kiro`; explicit Kiro model ID; workspace secret `KIRO_API_KEY`.
- No provisioned browser/device login. Local installs [pinned binary](../../../internal/core/runtime/versions.json).

## Runtime behavior

- V3 ACP harness, newline-delimited JSON-RPC.
- Per-session instructions/skills/model profile in private `KIRO_HOME`; MCP through session creation.
- `ask`: interactive capability confirmations; `autonomous`: native capability allow rules.
- Replay coverage remains unverified.

See [shared verification/recovery](README.md#verification-and-recovery).
