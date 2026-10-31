FROM node:22.22.2-bookworm-slim AS web-builder

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.2-bookworm AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/orkest ./cmd/orkest

FROM docker:27.5.1-cli AS docker-cli

FROM moby/buildkit:v0.34.1-rootless

USER root

COPY --from=go-builder /out/orkest /usr/local/bin/orkest
COPY --from=web-builder /src/web/dist /opt/orkest/web
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY build/docker/buildkitd.toml /etc/buildkit/buildkitd.toml

ENV HTTP_ADDR=0.0.0.0:8080 \
	ORKEST_WEB_DIR=/opt/orkest/web \
	ORKEST_BUILDCTL_BINARY=/usr/bin/buildctl-daemonless.sh \
	ORKEST_BUILDKIT_DAEMONLESS=true

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/orkest"]
