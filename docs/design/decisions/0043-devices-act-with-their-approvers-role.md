# 0043. A paired device acts with its approver's role

- **Date:** 2026-10-03
- **Status:** accepted
- **Supersedes:** the member cap on paired devices (#1659, 2026-09-28) and the "member-scoped …
  never gets an admin session" clause of [0023](0023-android-tv-watching-first.md)

## Context

On 2026-09-28 every paired device was capped at member, even when an admin paired it, because a TV
left in a shared room should not administer the house. The iPhone app (#1816) is where an admin
approves and denies channel requests away from a desk, and under the cap an admin's own phone could
not do that.

## Decision

A paired device acts as the person who approved it, with that person's real role, mapped exactly as
a session for them would be. An admin-paired phone or TV is an admin, and a member-paired device
stays a member. The role comes from the user's row on every request, never from the pairing, so a
demotion applies on the device's next request and a disabled or deleted user's devices stop
authenticating.

## Consequences

- An admin's iPhone can approve and deny channel requests and reach every admin route.
- A shared-room TV paired by an admin is an admin. Households that do not want that pair the TV
  from a member account, or revoke the device with **Disconnect this device** or from People.
- The Android TV client stays watching-first ([0023](0023-android-tv-watching-first.md)); it simply
  no longer relies on the server to keep an admin's TV from admin routes.
