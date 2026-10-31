
### BuildKit refinement

Image composition/publishing now lives in `internal/imagebuild/buildkit` and uses
standalone `buildctl` (tested v0.34.1). `internal/executors/docker/image_store.go`
only resolves registry inputs, pulls output artifacts, and validates them. The
worker's existing Builder interface and PostgreSQL persistence are unchanged.
The exporter digest is verified before promotion; metadata is kept outside the
build context. Registry input digests replace unsupported Docker-local aliases.
Configuration supports a remote endpoint and TLS files. A real integration test
uses a separate BuildKit daemon and verifies push, digest recovery, and Docker
execution; focused tests cover arguments, cancellation, bad metadata, and output
redaction. See the current operations guide for setup and verification results.

### Multiple builder backends

`ORKEST_IMAGE_BUILDER=buildkit|docker` selects a deployment-wide implementation of
`executionprofile.Builder`; BuildKit remains the default. Docker mode does not
initialize buildctl. Both adapters use `internal/imagebuild.Prepare` for identical
validated Dockerfiles/install resources and share Docker artifact verification.
Registry inputs and immutable output digests remain required in both modes.
The existing queue, leases, and reconciliation are unchanged. The composition
integration test now runs both backends, with standalone BuildKit setup confined
to its subtest so Docker-only verification has no BuildKit prerequisite.
