# Contributing to Orkest

Thank you for helping improve Orkest. Contributions should be focused, tested where practical, and consistent with the project's documented requirements and architecture.

## Before you start

Read these documents before making a change:

- [AGENTS.md](AGENTS.md) for repository conventions and verification requirements.
- [Project requirements](docs/requirements.md) for confirmed scope, proposed behavior, and open decisions.
- [Technology stack](docs/tech-stack.md) for architecture and implementation boundaries.

Do not commit credentials, generated secrets, local `.env` files, task content containing secrets, or ordinary logs containing sensitive data.

## Development setup

Install the frontend dependencies and configure local services:

```sh
cp .env.example .env
# Set POSTGRES_PASSWORD in .env.
make setup
make db-up
make auth-up
```

Run the backend with auto-reload:

```sh
make dev
```

For frontend work, run the backend in one terminal and the Vite development server in another:

```sh
make dev-web
```

See the [README](README.md) for prerequisites, environment variables, containerized development, and the local login account.

## Making changes

- Keep changes within the relevant package or documentation area.
- Preserve confirmed, proposed, and open distinctions in product and architecture documentation.
- Use explicit constructors and existing repository conventions for Go changes.
- Regenerate API artifacts with `npm --prefix web run generate:api` when the platform OpenAPI contract changes; do not edit generated artifacts manually.
- Add or update tests for observable behavior when changing application code.
- Update documentation when commands, configuration, behavior, or limitations change.

## Verification

Run the checks relevant to your change. The standard project check is:

```sh
make check
```

Useful targeted commands include:

```sh
make lint                         # Go lint and formatting checks
make test-postgres                # PostgreSQL repository tests
make test-runtime-conformance     # Runtime contracts and recovery checks
make test-recovery-unit           # Deterministic recovery tests with the race detector
npm --prefix web run test:e2e     # Playwright browser journeys
```

`make test-integration` requires the pinned OpenCode executable through `ORKEST_OPENCODE_BINARY`. `make test-recovery` additionally requires an explicit model, credentials, and provider environment. Report checks that could not run and the reason; do not describe them as passing.

## Pull requests

Before opening a pull request:

1. Review the diff for unrelated changes, secrets, and generated-file drift.
2. Run the relevant checks and record the commands and results in the pull request description.
3. Explain the user-visible behavior or documentation change.
4. Identify infrastructure, credentials, platform-version requirements, or other verification limitations.

Keep commits focused and use a concise conventional format such as `docs(readme): clarify local setup` or `fix(task): preserve run ownership`.

## Licensing

By contributing to Orkest, you agree that your contributions are provided under the [Apache License 2.0](LICENSE.md), unless a separate written agreement states otherwise.
