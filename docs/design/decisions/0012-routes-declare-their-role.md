# 0012. Every route declares its role; anonymous is denied by default

- **Date:** 2026-07-30
- **Status:** accepted

## Context

Authorization lived in `requireAdmin` calls at the top of handler bodies: 56 call sites against 83
operations. The other 27 checked no role, so they were reachable with no credential; one anonymous
route invoked the LLM. `RoleMember` existed but was never checked, and the test harness could not
express member-versus-anonymous.

## Decision

The required role is an argument when a route is registered, and middleware enforces it. A route with
no declared role is refused. Pre-authentication routes declare `RolePublic` explicitly. Routes mounted
outside the Huma middleware use one shared guard that fails closed.

## Consequences

- Forgetting a role closes a door instead of opening one.
- The public surface is a list found with one search, not an absence to infer.
- A test enumerates the registry and fails on any operation without a role.
