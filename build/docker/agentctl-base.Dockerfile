FROM golang:1.26.2-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentctl ./cmd/agentctl

FROM moby/buildkit:v0.34.1 AS buildkit

FROM node:22.19.0-bookworm-slim

ARG GH_VERSION=2.103.2
ARG GLAB_VERSION=1.125.0

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl git \
	&& case "$(dpkg --print-architecture)" in \
		amd64) gh_arch=amd64; gh_sha256=30463d8e1fdce73f178f76b0082b7ab0657b0740b87274890e831def647ca979; glab_sha256=cef937bc144078826c50f8dfa32fcd899a9a3e79dbffb0c8ed6e609a07b2ea55 ;; \
		arm64) gh_arch=arm64; gh_sha256=41290f26363155b0bef44a751d011994771f537e1f8c4253d1f53b5cf3b8641e; glab_sha256=2fdfeb0080ed33631176e9dc419ac10d52f1bda57b66238497f9c1f677a3f88f ;; \
		*) echo "unsupported architecture for GitHub CLI" >&2; exit 1 ;; \
	esac \
	&& curl -fsSL --retry 3 "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.deb" -o /tmp/gh.deb \
	&& curl -fsSL --retry 3 "https://gitlab.com/gitlab-org/cli/-/releases/v${GLAB_VERSION}/downloads/glab_${GLAB_VERSION}_linux_${gh_arch}.deb" -o /tmp/glab.deb \
	&& echo "${gh_sha256}  /tmp/gh.deb" | sha256sum -c - \
	&& echo "${glab_sha256}  /tmp/glab.deb" | sha256sum -c - \
	&& apt-get install -y --no-install-recommends /tmp/gh.deb /tmp/glab.deb \
	&& gh --version | head -n 1 | grep -F "gh version ${GH_VERSION}" \
	&& glab --version | grep -F "${GLAB_VERSION}" \
	&& rm -f /tmp/gh.deb /tmp/glab.deb \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/agentctl /usr/local/bin/agentctl
COPY --from=buildkit /usr/bin/buildctl /usr/local/bin/buildctl
RUN buildctl --version | grep -F "v0.34.1"
ENTRYPOINT ["/usr/local/bin/agentctl"]
