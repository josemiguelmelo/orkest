# Webapp development

React uses the authenticated platform API, never worker endpoints. Comments/chat
are separate. User guides: [sessions](../guides/sessions.md), [workflows](../guides/workflows.md), [artifacts](../guides/artifacts.md).

## Modules and UI

- Versions: [package.json](../../web/package.json); build: Vite.
- TanStack Router: typed navigation/parameters/URL filters; Query: server state.
- React Hook Form/Zod: forms.

| Directory | Owns |
| --- | --- |
| `web/src/app` | Shell, permissions, context, routing |
| `web/src/features` | Domain behavior, forms, query usage |
| `web/src/shared` | Generated API library, query factories, reusable UI |

- Add feature modules/routes, not generic CRUD/plugin frameworks.
- Owned shadcn-style Button/Dialog: Radix, class-variance-authority, Tailwind;
  `components.json` layout, CSS-variable colors/system light-dark themes.
- Radix focus handling; narrow-screen navigation/board scrolling; Markdown excludes raw HTML.

## API generation

- Pinned Hey API generates SDK, TypeScript DTOs, request/response Zod validators from
  platform OpenAPI. Never hand-edit output; [commands](testing.md#frontend-and-generated-artifacts).
- Bundled Fetch lacks `exactOptionalPropertyTypes` support; compile separately with
  standard strict TypeScript to committed JavaScript/declarations. Ignore temporary `.api-source`.
- App retains `strict`, `exactOptionalPropertyTypes`, `noUncheckedIndexedAccess`, typed
  unsafe-operation lint. Compile-only negative assertions check route/API/input boundaries.

## State and credentials

- Query keys include user/applicable organization/workspace. Navigation cancels
  outstanding organization requests; logout/expiry clears caches.
- Mutations use transient local state, excluding write-only secrets from cached
  variables; never auto-retry.
- Catalog-supplied provider JSON: string/string-array object renderer; Ajv 2020 constraints.
  Unsupported constructs fail explicitly; supported providers need no core UI change.
  Trigger editors omit retired settings when saving against the current descriptor.
- MCP headers: preserve/replace/clear. Saves omit unchanged integration credentials;
  validation requires new credentials. Forms discard secrets; never log/store in browser storage.

## Activity and deployment

- Authenticated task endpoints: run metadata every two seconds, ordered activity
  every 1.5 seconds from persisted history. Approvals bind run/session/request/option.
- Go serves `ORKEST_WEB_DIR` (default `web/dist`): SPA deep links; unknown API/auth/docs
  and missing assets remain errors. API works without frontend build.
- Deploy assets beside binary; no embedding.

## Development

1. [Quick start](../../README.md#run-locally): dependencies, binaries, `web/dist`.
   `make dev-once` serves build; backend changes require restart or Air (`make dev`).
2. Keep API running; `make dev-web` serves `http://127.0.0.1:5173`, proxying `/api`/`/auth`
   to `127.0.0.1:8080`. Before backend start, set:

```dotenv
OIDC_REDIRECT_URL=http://127.0.0.1:5173/auth/callback
```

- Use one hostname: cookies are host-scoped. Realm import allows localhost/127.0.0.1
  callbacks on 8080/5173; existing realms need manual client URI updates, not import-file edits.
- [Testing](testing.md#frontend-and-generated-artifacts): formatting, generation, mock
  browser journeys, opt-in live Keycloak/API/SPA smoke. Mocks prove no live integrations/agents;
  past counts/audits are not current guarantees.
