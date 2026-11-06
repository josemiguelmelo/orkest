# Image build adapters

- `executionprofile.Builder`: the consumer-owned port in `internal/server/executionprofile`;
  `cmd/server` selects BuildKit or Docker implementations.
- Both use `Prepare` in `internal/adapters/imagebuild` for validated Dockerfiles/runtime resources.
- Docker storage resolves inputs/validates outputs; metadata stays outside context;
  check exporter digests before promotion.

```mermaid
flowchart LR
    Save[Save profile] -->|same transaction| Job[(Build job)]
    Job -->|claim lease| Worker[Build worker]
    Worker --> Backend[BuildKit or Docker adapter]
    Backend -->|publish digest| Registry[(Registry)]
    Registry -->|pull and validate| Validate[Docker image validation]
    Validate -->|matching generation only| Ready[Ready profile image]
```

## Jobs and recovery

- Profile changes/jobs commit together. `SKIP LOCKED` claims, 90-second leases renewed
  every 20 seconds, 30-minute timeout. Configured workers run per process; multiple
  processes can build different profiles concurrently.
- BuildKit: build/push → Docker pull exported digest → runtime/writable-directory validation.
- Docker: validate locally → push → verify published digest.
- Ready only after validation; generation checks prevent stale promotion.
  Discard raw output; sanitize persisted errors.
- Expired lease: inspect/validate attempt's published image before retry. Unresolved
  outcomes become interrupted, requiring explicit retry.
- Shutdown joins builder without stopping agents. Client disconnection proves no daemon termination.

See [builder operations](../operations/image-builds.md) and [testing](testing.md).
