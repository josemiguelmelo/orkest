# Toolchain image for agents developing the Orkest repository.
# Pass a published agentctl-base image by immutable registry digest.
ARG ORKEST_AGENTCTL_BASE_IMAGE
FROM ${ORKEST_AGENTCTL_BASE_IMAGE}

COPY --from=golang:1.26.2-bookworm /usr/local/go /usr/local/go
COPY --from=node:22.22.2-bookworm-slim /usr/local/bin/ /usr/local/bin/
COPY --from=node:22.22.2-bookworm-slim /usr/local/lib/node_modules/ /usr/local/lib/node_modules/

ENV PATH="/usr/local/go/bin:${PATH}"

USER root

RUN apt-get update \
	&& apt-get install -y --no-install-recommends build-essential make python3 \
	&& rm -rf /var/lib/apt/lists/* \
	&& go version | grep -F 'go1.26.2' \
	&& node --version | grep -F 'v22.22.2' \
	&& npm --version \
	&& python3 --version \
	&& make --version | head -n 1

# The base image's default user and entrypoint are preserved.
