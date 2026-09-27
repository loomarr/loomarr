# 0011. SSO is a credential path, not a provisioning path

- **Date:** 2026-07-30
- **Status:** accepted

## Context

Households that already run an identity provider want single sign-on (V8). Common SSO integrations
create accounts on first sign-in and map provider groups to roles, which would bypass the allowlist
([0007](0007-loomarr-owns-identity.md)).

## Decision

OIDC is a third credential path onto the same `users` table. An SSO identity without a row is
rejected, nothing is created on first sign-in, and roles are never derived from provider groups.
Forward-auth header trust is not built: a Loomarr reachable beside its proxy would accept any
`Remote-User` header as identity, while OIDC carries its own signed proof.

## Consequences

- Admins still import or create every user; SSO only changes how they prove who they are.
- Loomarr's own sign-in and the break-glass `API_TOKEN` keep working when the provider is down.
- The flow is verified against real providers (`make test-sso`), not a stub.
