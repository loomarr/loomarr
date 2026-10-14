---
description: Find capabilities that exist in the API but have no way to reach them
---

# Surface audit

Find capabilities the backend supports that no user can reach. This repo produces this
defect reliably: `policy.autoCurate` shipped complete and live-verified with no opt-in
toggle; `strategy` and `group` are PATCHable with no UI; season windows carry a doc-string
promising the UI can edit them and render as a read-only badge; `GET /v1/backup` and
`GET /v1/system/version` are both implemented with no UI at all.

Each of those cleared its phase gate. The gate asked whether the code worked, not whether
anyone could get to it.

**Input:** a domain to scope the audit (`channels`, `settings`, `people`,
`filler`, `guide`). Optional, but a whole-API sweep is shallow; prefer a domain.

## Procedure

This is a fan-out — each capability is checked independently, so work through them
mechanically rather than reasoning about the domain as a whole.

**1. Enumerate the supply.** From `api/openapi.yaml` (authoritative and generated — do not
read the handlers for this): every endpoint in scope, and every **field of every request
body**. Fields matter more than endpoints here: `PATCH /v1/channels/{id}` is obviously
reachable, while `strategy` inside its body is the thing nobody can set.

Add: settings keys from `internal/settings/declared.go`, and generated secrets from
`internal/settings/secrets.go`.

**2. Find the demand.** For each, search `web/apps/web/src` for a component that reads or
writes it. Trace to a **route** in `src/routes/` — a component that nothing renders is not
a door.

**3. Classify.**

- **REACHABLE** — a user can get to it; name the route
- **ORPHANED** — no UI path exists, and no doc says that is intentional
- **API-ONLY** — no UI, but a doc explicitly marks it API-only (this is a legitimate answer)
- **PARTIAL** — readable but not writable, or writable only through a flow that cannot be
  reached directly. Season windows are the worked example: displayed, documented as
  editable, not editable

## Rules

- **A test is not a door.** Nor is a Storybook story, nor a type definition. Only a
  component reachable from a route.
- **Check the writable direction.** Many fields render fine and cannot be changed. PARTIAL
  is the most commonly missed verdict.
- **Look for orphaned exports.** A component nothing imports is dead weight the
  story-coverage gate will keep demanding stories for.
- **Cite both sides.** `field → openapi.yaml:LINE → component:LINE → route`, or the point
  where the trail goes cold.

## Output

```
SURFACE AUDIT: <domain>
reachable: <n>   orphaned: <n>   api-only: <n>   partial: <n>

ORPHANED
  <capability>   <api ref>
      no UI path found; searched <what you searched>

PARTIAL
  <capability>   <api ref>
      reads: <component:line>     writes: none
```

Finish by proposing the **surface-map rows** these findings imply — `capability → API field
→ UI location → audience` — for design.md §12's channel surface map, or the equivalent
table for the domain. An orphan resolves one of two ways: build the door, or write the row
saying it is deliberately API-only. Both are valid; silence is not.
