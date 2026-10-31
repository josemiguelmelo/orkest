# Sample toolchain image for Orkest execution profiles.
# Pass a published orkest/agentctl-base image by immutable registry digest.
#ARG ORKEST_AGENTCTL_BASE_IMAGE
FROM localhost:5001/orkest/agentctl-base:dev

USER root

ENV TERRAFORM_VERSION=1.12.2

RUN set -eu; \
	case "$(dpkg --print-architecture)" in \
	amd64) terraform_arch=amd64; terraform_sha256=1eaed12ca41fcfe094da3d76a7e9aa0639ad3409c43be0103ee9f5a1ff4b7437 ;; \
	arm64) terraform_arch=arm64; terraform_sha256=f8a0347dc5e68e6d60a9fa2db361762e7943ed084a773f28a981d988ceb6fdc9 ;; \
	*) echo "unsupported architecture for Terraform" >&2; exit 1 ;; \
	esac; \
	curl -fsSL --retry 3 "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_${terraform_arch}.zip" -o /tmp/terraform.zip; \
	echo "${terraform_sha256}  /tmp/terraform.zip" | sha256sum -c -; \
	apt-get update; \
	apt-get install -y --no-install-recommends unzip; \
	unzip -q /tmp/terraform.zip -d /usr/local/bin; \
	terraform version | grep -F "Terraform v${TERRAFORM_VERSION}"; \
	rm -f /tmp/terraform.zip; \
	apt-get purge -y --auto-remove unzip; \
	rm -rf /var/lib/apt/lists/*

# The base image's default user and entrypoint are preserved.
