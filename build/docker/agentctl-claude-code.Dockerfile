ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

COPY internal/core/runtime/versions.json /tmp/orkest-runtime-versions.json
RUN runtime_version="$(node -p 'require("/tmp/orkest-runtime-versions.json")["claude-code"]')" \
    && test -n "$runtime_version" \
    && npm install --global "@agentclientprotocol/claude-agent-acp@${runtime_version}" \
    && test "$(claude-agent-acp --version)" = "${runtime_version}" \
    && rm /tmp/orkest-runtime-versions.json
