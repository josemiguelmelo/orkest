# Task editor image pair; editor and readonly targets must remain distinct.
FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc AS runtime

# Checksums from the GitHub release asset digests, not a downloaded checksum file.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates=20250419~deb12u2 curl=7.88.1-10+deb12u15 \
        git=1:2.39.5-0+deb12u3 bash=5.2.15-2+b13 procps=2:4.0.2-3 \
    && case "$(dpkg --print-architecture)" in \
        amd64) arch=amd64; checksum=2f6db6c3e8b733dd6b3aeee83532e148b2bae2e392353ba85452d03572348962 ;; \
        arm64) arch=arm64; checksum=e867a34416e2da4a83496b09580001893e7a4e5bb58a0624c4fcd22819e7d53d ;; \
        *) exit 1 ;; \
    esac \
    && curl -fsSL --retry 3 "https://github.com/coder/code-server/releases/download/v4.145.2/code-server-4.145.2-linux-${arch}.tar.gz" -o /tmp/code-server.tar.gz \
    && echo "${checksum}  /tmp/code-server.tar.gz" | sha256sum -c - \
    && mkdir /opt/code-server /opt/editor /opt/extensions \
    && tar -xzf /tmp/code-server.tar.gz --strip-components=1 -C /opt/code-server \
    && /opt/code-server/bin/code-server --version \
    && install -d -o 1000 -g 1000 /home/editor \
    && rm /tmp/code-server.tar.gz \
    && rm -rf /var/lib/apt/lists/* /root/.config

COPY tools/task-editor/entrypoint.sh /opt/editor/
COPY tools/task-editor/extensions.json /opt/extensions/extensions.json
ENV HOME=/home/editor
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/bin/bash", "/opt/editor/entrypoint.sh"]

# Enforce terminal denial independently of child-process environment filtering.
FROM runtime AS readonly
COPY tools/task-editor/restrict.cjs /opt/code-server/lib/vscode/node_modules/node-pty/lib/index.js

FROM runtime AS editor
