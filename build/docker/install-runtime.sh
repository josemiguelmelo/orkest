#!/bin/sh
set -eu
runtime="$1"
version="$2"
case "$runtime" in
  opencode) npm install --global "@opencode/cli@${version}"; test "$(opencode --version)" = "opencode v${version}" ;;
  claude-code) npm install --global "@agentclientprotocol/claude-agent-acp@${version}"; test "$(claude-agent-acp --version)" = "${version}" ;;
  codex) npm install --global "@agentclientprotocol/codex-acp@${version}"; test "$(codex-acp --version)" = "@agentclientprotocol/codex-acp ${version}" ;;
  *) exit 1 ;;
esac
