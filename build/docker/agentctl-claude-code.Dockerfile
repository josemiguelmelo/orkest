ARG AGENTCTL_BASE_IMAGE=orkest/agentctl-base:dev
FROM ${AGENTCTL_BASE_IMAGE}

ARG CLAUDE_AGENT_ACP_VERSION=0.99.0
RUN npm install --global "@agentclientprotocol/claude-agent-acp@${CLAUDE_AGENT_ACP_VERSION}" \
    && test "$(claude-agent-acp --version)" = "${CLAUDE_AGENT_ACP_VERSION}"
