FROM golang:1.26.2-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentctl ./cmd/agentctl

FROM node:22.19.0-bookworm-slim

ARG GH_VERSION=2.103.2

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl git \
	&& case "$(dpkg --print-architecture)" in \
		amd64) gh_arch=amd64; gh_sha256=30463d8e1fdce73f178f76b0082b7ab0657b0740b87274890e831def647ca979 ;; \
		arm64) gh_arch=arm64; gh_sha256=41290f26363155b0bef44a751d011994771f537e1f8c4253d1f53b5cf3b8641e ;; \
		*) echo "unsupported architecture for GitHub CLI" >&2; exit 1 ;; \
	esac \
	&& curl -fsSL --retry 3 "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.deb" -o /tmp/gh.deb \
	&& echo "${gh_sha256}  /tmp/gh.deb" | sha256sum -c - \
	&& apt-get install -y --no-install-recommends /tmp/gh.deb \
	&& gh --version | head -n 1 | grep -F "gh version ${GH_VERSION}" \
	&& rm -f /tmp/gh.deb \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/agentctl /usr/local/bin/agentctl
ENTRYPOINT ["/usr/local/bin/agentctl"]
