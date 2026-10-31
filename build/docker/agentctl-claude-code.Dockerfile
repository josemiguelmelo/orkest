ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

COPY internal/core/runtime/versions.json /tmp/orkest-runtime-versions.json
COPY build/docker/install-runtime.sh /tmp/install-runtime.sh
RUN runtime_version="$(node -p 'require("/tmp/orkest-runtime-versions.json")["claude-code"].replace(/^opencode v/, "")')" \
    && sh /tmp/install-runtime.sh claude-code "$runtime_version" \
    && rm /tmp/install-runtime.sh /tmp/orkest-runtime-versions.json
