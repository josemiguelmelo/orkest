FROM golang:1.26.2-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentctl ./cmd/agentctl

FROM node:22.19.0-bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/agentctl /usr/local/bin/agentctl
ENTRYPOINT ["/usr/local/bin/agentctl"]
