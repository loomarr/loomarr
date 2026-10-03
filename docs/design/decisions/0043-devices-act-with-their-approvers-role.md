# 0043. A paired device acts with its approver's role

- **Date:** 2026-10-03
- **Status:** accepted
- **Supersedes:** the member cap on paired devices (#1659, 2026-09-28), and in part
  [0023](0023-android-tv-watching-first.md): its clause that the TV client is "member-scoped" and
  "never gets an admin session". The rest of 0023 stands.

## Context

On 2026-09-28 every paired device was capped at member, even when an admin paired it, because a TV
left in a shared room should not administer the house. The iPhone app (#1816) is where an admin
approves and denies channel requests away from a desk, and under the cap an admin's own phone could
not do that.

## Decision

A paired device acts as the person who approved it, with that person's real role, mapped exactly as
a session for them would be. An admin-paired phone or TV is an admin, and a member-paired device
stays a member. The role comes from the user's row on every request, never from the pairing, so a
demotion applies on the device's next request. Disabling a user deletes their paired devices along
with their sessions, so re-enabling the user does not bring a lost device back.

## Consequences

- An admin's iPhone can approve and deny channel requests and reach every admin route.
- An admin device can do anything an admin session can, including revealing or regenerating
  redacted settings secrets (`API_TOKEN` among them) and approving further pairings as its user.
  Revoking a lost admin device does not undo a secret it already read, so rotate secrets too; any
  device it paired shows up in the owner's paired-devices list.
- A shared-room TV paired by an admin is an admin. Households that do not want that pair the TV
  from a member account. A device is revoked with **Disconnect this device** on the device itself,
  from its owner's paired-devices list in Settings, or by disabling the user, which revokes all of
  their devices. There is no admin view of another user's devices.
- The Android TV client stays watching-first ([0023](0023-android-tv-watching-first.md)); it simply
  no longer relies on the server to keep an admin's TV from admin routes.
