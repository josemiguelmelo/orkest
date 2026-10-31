#!/bin/sh
set -eu
runtime="$1"
version="$2"
case "$runtime" in
  opencode) npm install --global "@opencode/cli@${version}"; test "$(opencode --version)" = "opencode v${version}" ;;
  claude-code) npm install --global "@agentclientprotocol/claude-agent-acp@${version}"; test "$(claude-agent-acp --version)" = "${version}" ;;
  codex) npm install --global "@agentclientprotocol/codex-acp@${version}"; test "$(codex-acp --version)" = "@agentclientprotocol/codex-acp ${version}" ;;
  github-copilot) npm install --global "@github/copilot@${version}"; test "$(copilot --version | head -n 1)" = "GitHub Copilot CLI ${version}." ;;
  kiro) \
    test "$version" = "2.37.0"; \
    case "$(dpkg --print-architecture)" in \
      amd64) filename="kirocli-x86_64-linux.tar.gz"; checksum="02c49873ba64285f34048aac377ccf0b546b5627f39dd775bae9fa306112ff19" ;; \
      arm64) filename="kirocli-aarch64-linux-musl.tar.gz"; checksum="945b334790d5a12c9953f1ba01846fa1274c2d28e90a66ee023a9972a063e574" ;; \
      *) echo "unsupported architecture for Kiro CLI" >&2; exit 1 ;; \
    esac; \
    archive="/tmp/kiro-cli.tar.gz"; \
    curl -fsSL "https://prod.download.cli.kiro.dev/stable/$version/$filename" -o "$archive"; \
    echo "$checksum  $archive" | sha256sum -c -; \
    mkdir /tmp/kiro-cli; \
    tar -xzf "$archive" -C /tmp/kiro-cli; \
    Q_INSTALL_GLOBAL=1 Q_SKIP_SETUP=1 /tmp/kiro-cli/kirocli/install.sh; \
    rm -rf /tmp/kiro-cli "$archive"; \
    kiro-cli --version | grep -F "$version" ;;
  *) exit 1 ;;
esac
