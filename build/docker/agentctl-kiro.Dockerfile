ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

COPY internal/core/runtime/versions.json /tmp/orkest-runtime-versions.json
COPY build/docker/install-runtime.sh /tmp/install-runtime.sh
RUN runtime_version="$(node -p 'require("/tmp/orkest-runtime-versions.json")["kiro"]')" \
    && sh /tmp/install-runtime.sh kiro-cli "$runtime_version" \
    && rm /tmp/install-runtime.sh /tmp/orkest-runtime-versions.json
