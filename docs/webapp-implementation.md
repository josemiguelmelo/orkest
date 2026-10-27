# Management webapp

## Implemented behavior

The compact workbench covers organizations, workspaces and memberships; tasks, comments and components; agent definitions; global/organization/workspace skills; the existing GitHub URL repository registration flow; task integrations; HTTP MCP connections; and task run activity, chat, approval responses, and cancellation. Replacing the URL flow with coding integration management and a paginated repository picker remains pending. All screens use the authenticated platform API. The standalone agentctl API is not exposed.

Users can create tasks in todo, assign agents, and explicitly start work. Task detail shows durable launch history and ordered runtime activity, polls for new activity, and supports chat, permission responses, and cancellation while a run is active. Agent chat remains separate from user comments. The run API checks task and workspace access, rejects viewer mutations, and does not expose executor endpoints or process identities. The platform currently closes the agent process when a prompt completes, so further chat is only available during that active work step. Team member slot allocation and supported session continuation are not implemented; the UI does not claim either capability. Done tasks cannot reopen. Imported title, description and labels stay source-managed.

Organization roles and effective workspace roles control actions; the backend remains authoritative. Organization Owners/Admins inherit workspace access. Viewers are read-only. Integration configuration and scoped skill management use their existing administrator rules. Only platform administrators can manage global skills; `/api/me` now exposes the existing `platformAdmin` permission. Organization membership creation accepts an existing internal user ID; invitations and global user search are not implemented.

## Architecture decisions

- Retain React 19, TypeScript 5.9 and Vite. Use TanStack Router for typed navigation, path parameters and validated URL filters; TanStack Query for server state; React Hook Form and Zod for forms.
- `web/src/app` owns shell, permissions, context and routing. Domain modules in `features` own behavior, forms and query usage. `shared` contains the generated API library, query factories and reusable UI. Add ordinary feature modules and routes rather than a generic CRUD or plugin framework.
- Own shadcn-style Button/Dialog primitives built with Radix, class-variance-authority and Tailwind v4. `components.json` records the component layout. CSS variables define shared colors and system-aware light/dark themes. Markdown rendering omits raw HTML. Dialog focus handling comes from Radix; mobile navigation and board scrolling support narrow screens.
- Generate SDK functions, TypeScript DTOs and request/response Zod validators from `internal/app/httpapi/openapi.yaml`, using pinned Hey API. Never manually edit generated files. The upstream bundled Fetch implementation does not compile with `exactOptionalPropertyTypes`; generation therefore compiles that library with standard strict TypeScript into JavaScript and declarations. Application code still enables `strict`, `exactOptionalPropertyTypes`, `noUncheckedIndexedAccess` and typed unsafe-operation lint checks. Compile-only negative assertions verify route/API/input boundaries.
- Query keys include user and applicable organization/workspace IDs. Scoped navigation cancels outstanding organization requests. Logout/session expiry clears cached data. Mutations use transient local state rather than the mutation cache so write-only credentials are not retained in cached variables. Mutations are never automatically retried.
- Task run history and ordered activity use authenticated task-scoped endpoints. The UI polls run metadata every two seconds and activity every 1.5 seconds, resuming from the persisted event history returned by the platform. Permission responses carry the run, session, request, and selected option identities.
- Provider configuration remains dynamic JSON validated against the authenticated provider catalog. A small renderer supports current string/string-array object schemas. Ajv 2020 validates published constraints. Unsupported schema constructs fail explicitly; they are not silently ignored. New providers using supported constructs need no core screen changes.
- MCP header editing distinguishes preserve, replace and clear. Integration edits omit unchanged credentials. Explicit connection validation needs a newly supplied credential; save can preserve stored credentials. Secret inputs disappear with their forms and are never persisted to browser storage or logged.
- Serve the built SPA from Go using `ORKEST_WEB_DIR` (default `web/dist`). Deep links return the SPA; unknown API/auth/docs routes and missing assets remain errors. The API remains available without a frontend build. Deploy the compiled asset directory alongside the Go binary; the binary does not embed it.

## Development and verification

`make setup` installs pinned npm dependencies. `make build` produces both Go binaries and `web/dist`; `make dev-once` then serves the UI at the configured Go address. An already running binary must restart to load backend changes.

For frontend iteration, run the API and `make dev-web`. Vite proxies `/api` and `/auth` to `127.0.0.1:8080`. Set the backend's `OIDC_REDIRECT_URL` to the exact frontend origin, for example `http://127.0.0.1:5173/auth/callback`, before starting it. The realm import includes localhost and 127.0.0.1 callbacks on 8080 and 5173. Existing Keycloak realms are not updated by editing the import file: add the matching redirect URI to the existing local client configuration. Use the same hostname throughout login; cookies are host-scoped.

Commands:

- `make check`: Go lint/tests, API generation drift check, frontend lint/unit tests/type checks, and all builds. Go tooling targets project Go trees explicitly so Go files shipped by npm packages are not included.
- `npm --prefix web run generate:api`: regenerate committed SDK JavaScript/declarations and validators. Temporary generator source stays in ignored `.api-source`.
- `npm --prefix web run check:generated`: generate separately and compare without changing committed output.
- `npm --prefix web run format` / `format:check`: frontend formatting.
- `npm --prefix web run test:e2e`: desktop and mobile Chromium journeys against typed API mocks. Install the pinned browser with `web/node_modules/.bin/playwright install chromium` first.
- `make test-postgres`: real repository suite with documented PostgreSQL settings.
- `node web/scripts/smoke-live.mjs`: opt-in real Keycloak/API/SPA smoke journey. Run only against a disposable local database. It creates a dedicated organization, exercises management resources and removes that organization. Configure `ORKEST_LIVE_BASE_URL`, optionally `ORKEST_LIVE_LOGIN_URL`, `ORKEST_LIVE_USERNAME` and `ORKEST_LIVE_PASSWORD`; defaults target the documented local setup. It never starts an agent or clones remote code. Screenshots are written to the OS temporary directory.

## Verification record

- `make check`: passed, including Go routing/platform-admin tests and 20 frontend behavioral tests.
- Desktop/mobile Chromium: 10 journeys passed, including task creation, deep-link refresh, URL filters, viewers, workspace switching, session expiry and dialog focus/Escape.
- Real PostgreSQL repository suite: passed against an isolated local database.
- Real local Keycloak and Go-served SPA: login/logout, organization/workspace creation, task/comment persistence after reload, agent and MCP creation verified.
- Extended real-backend smoke: skills, repository registration, components and the actual GitHub/Jira provider catalog forms passed. Desktop/mobile screenshots were visually reviewed. Test organizations were removed.
- Dependency audit: zero known vulnerabilities with the pinned transitive YAML fix. Checks ran on Node 26.0.0; the documented Node 22 baseline is updated to 22.22.2 to satisfy the test tooling.

External GitHub/Jira synchronization with real credentials, private repositories, agent execution/approvals and OpenCode recovery are not claimed as verified. The existing recovery suite remains unimplemented. No migration is required by this change.
