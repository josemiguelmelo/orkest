# Task editor image support files

These files are copied into the pinned task editor images by
[`build/docker/task-editor.Dockerfile`](../../build/docker/task-editor.Dockerfile).
They provide the container entrypoint, native-host gateway relay, and the
read-only image's PTY restriction.

Build images with `make docker-task-editor-images`. Run Docker lifecycle and
browser qualification with `make test-task-editor-backend`; security probes use
`make test-task-editor-security`. See
[`task-editor-backend.md`](../../docs/development/task-editor-backend.md) for
deployment constraints and verification requirements. The original feasibility
prototype and its observations remain in the task editor delivery history.
