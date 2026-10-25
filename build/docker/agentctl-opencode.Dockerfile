ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

ARG OPENCODE_VERSION=2.1.4
RUN npm install --global "@opencode/cli@${OPENCODE_VERSION}" \
    && test "$(opencode --version)" = "opencode v${OPENCODE_VERSION}"
