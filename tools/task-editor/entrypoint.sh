#!/bin/bash
set -euo pipefail
mkdir -p "$HOME/.local/share/code-server/User" "$HOME/extensions"
cat > "$HOME/.local/share/code-server/User/settings.json" <<'JSON'
{
  "workbench.startupEditor": "none",
  "security.workspace.trust.enabled": false,
  "task.allowAutomaticTasks": "off",
  "git.openRepositoryInParentFolders": "never",
  "git.autofetch": false,
  "telemetry.telemetryLevel": "off",
  "extensions.autoCheckUpdates": false,
  "extensions.autoUpdate": false,
  "terminal.integrated.enablePersistentSessions": false,
  "editor.minimap.enabled": false,
  "chat.disableAIFeatures": true,
  "window.dialogStyle": "custom"
}
JSON
args=(--auth none --bind-addr 0.0.0.0:8080 --disable-telemetry --disable-update-check --disable-proxy --disable-file-cache --disable-workspace-trust)
if [[ "${EDITOR_MODE:?set EDITOR_MODE to rw or ro}" == ro ]]; then
  args+=(--extensions-dir /opt/extensions --vscode-option disable-extensions)
elif [[ "$EDITOR_MODE" != rw ]]; then
  exit 1
fi
exec /opt/code-server/bin/code-server "${args[@]}" /workspace/task.code-workspace
