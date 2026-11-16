# Authentication

Username/password and Keycloak OIDC sign-in are always enabled. Keycloak must be
configured and available at startup. Set `ORKEST_COOKIE_SECURE=true` when serving
over HTTPS.

The first local platform administrator is ensured automatically. By default its
username and password are both `admin`, with email `admin@orkest.local`. Override
the bootstrap settings before startup; an existing account's password is never
reset on later starts. Replace the default password before exposing Orkest.
Provisioned accounts are normal users; platform-admin status requires a separate
promotion.

Local accounts are provisioned by organization Owners/Admins or platform
administrators from organization settings. Provisioning requires an explicit
organization role and an initial password of at least 12 characters. It creates
a normal account; platform-admin status is a separate promotion available only
to an existing platform administrator. Users with a local password can change
it from User settings after entering their current password. Password recovery
and self-registration are not available. Keycloak-only accounts manage their
password through Keycloak.

Local passwords are stored using Argon2id. Browser sessions use random opaque
tokens; only token hashes are persisted, and logout revokes the current local
session. The browser cookie is HttpOnly and SameSite=Lax. Configure secure
cookies for HTTPS deployments.
The password endpoint allows ten attempts per source IP in a rolling 15-minute
window per server process. Deployments with multiple server replicas should
enforce a shared edge rate limit as well.

When a local user later signs in through Keycloak, Orkest links the Keycloak
subject to that user only when the provider asserts a verified email matching
exactly one local account. Ambiguous matches and existing identity conflicts are
rejected. Orkest does not merge user records or memberships.

Migration `0053` backfills Keycloak identity mappings without changing user IDs.
Its downgrade requires local accounts without a linked Keycloak identity to be
removed or linked first because the original schema requires every user to have
a Keycloak subject.
