# Kiro CLI runtime

## Configuration

- Runtime/provider `kiro`; explicit Kiro model ID; workspace secret `KIRO_API_KEY`.
- No provisioned browser/device login. Local installs [pinned binary](../../../internal/contracts/runtimecatalog/versions.json).

## Runtime behavior

- V3 ACP harness, newline-delimited JSON-RPC.
- Per-session instructions/skills/model profile in private `KIRO_HOME`; MCP through session creation.
- `ask`: interactive capability confirmations; `autonomous`: native capability allow rules.
- Replay coverage remains unverified.

The configured model must also be confirmed through ACP before prompts on both
create and restore. Unsupported confirmation preserves state and fails visibly.
Deterministic conformance is covered; live model confirmation and continuation
remain unverified for this provider. See [shared verification/recovery](README.md#verification-and-recovery)
and [runtime qualification](../../development/testing.md#runtime-qualification).
