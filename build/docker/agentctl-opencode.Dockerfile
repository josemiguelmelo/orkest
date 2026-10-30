ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

COPY internal/core/runtime/versions.json /tmp/orkest-runtime-versions.json
RUN runtime_version="$(node -p 'require("/tmp/orkest-runtime-versions.json")["opencode"].replace(/^opencode v/, "")')" \
    && test -n "$runtime_version" \
    && npm install --global "@opencode/cli@${runtime_version}" \
    && test "$(opencode --version)" = "opencode v${runtime_version}" \
    && rm /tmp/orkest-runtime-versions.json
