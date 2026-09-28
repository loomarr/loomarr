# Acquisition

Formerly `design.md` §3, §4 and the requester and client-resilience parts of §6. How Loomarr gets a
title it wants into the media library and tracks it until it is there or given up on.

## Titles and keys

A **Title** is content Loomarr wants. Its identity is an external id, never a name.

- `MediaType`: `movie` or `series`.
- `TMDBID` (canonical for movies, accepted by Seerr for series) and `TVDBID` (preferred for series).
- `Name` and `Year` appear in logs and request payloads only.
- `Seasons []int` for series; empty means all.

The **Key** is the same whether it comes from a Title or a library scan item: `series:tvdb:<id>` when
a series has a TVDB id, otherwise `<mediatype>:tmdb:<id>`.

A **Record** is the persisted state: key, title, state, library item id, `requested_at`, `deadline`,
`attempts`, `last_error`, `updated_at`, plus download `progress`, `eta_text` and `download_status`.
Timestamps are Unix-epoch `BIGINT` so the schema stays dialect-neutral.

## State machine

![Acquisition state transitions from request submission through availability or terminal failure](../diagrams/generated/acquisition-state.svg)

*[D2 source](../diagrams/acquisition-state.d2)*

| State | Meaning | Emits an event |
| --- | --- | --- |
| `wanted` | requested by Loomarr; not yet accepted downstream | no |
| `requested` | accepted by Seerr or Sonarr/Radarr; awaiting a release | no |
| `downloading` | a release was grabbed and is in flight | no |
| `available` | present in the library and schedulable | yes, to the scheduler |
| `unavailable` | given up (deadline passed or unfindable) | yes, to the scheduler |

**Availability is discovered by polling, never by an inbound webhook.** The `library-scan` job lists
what the media server recently added and applies `LibraryConfirmed` to any in-flight title it finds;
`library-full-scan` runs less often as a safety net for anything the incremental window missed. A
scanned item is matched by every key it can produce, not only its preferred one: a series added
TMDB-only is keyed `series:tmdb:<id>` while the server exposes both ids on the show, so probing only
the TVDB key would never confirm it. The queue pollers (`arr-queue-poll`, `seerr-queue-poll`) move a
title with a live download to `downloading` (`Grabbed`) and record its progress.

**Arrival.** `LibraryConfirmed` stamps the record's `availableAt`, once (the state is terminal after
it). That is Home's **New this week** (#1663): `GET /v1/titles?since=<ms>` lists titles that arrived
from then, newest first, each with the channels whose lineup holds it (detached channels excluded;
one channel read, joined server-side). A title a channel picked from the library is written straight
to `available` with no stamp: it was already there, so it never reads as new, and a later unstamped
write keeps an earlier arrival. A channel likewise carries `createdAtMs` (stamped on insert, never
moved) and `requestedBy`, the name of the person whose request produced it, read through its job.

### Invariants

1. **Terminal states do not regress** within the acquisition lifecycle. `available` describes a
   moment, not forever: media gets deleted or re-identified, so the scheduler revalidates lineup items
   against the library at reconcile time instead of mutating terminal state.
2. **Only `available` and `unavailable` emit events**, the only transitions the scheduler acts on.
3. **Enqueue is idempotent**: deduplicated by external id in the store, with a per-key lock against
   concurrent double requests.
4. **The library is the source of truth.** A title is `available` only once the library reports it.
5. **Every in-flight record has a deadline.** A grab resets it to the shorter downloading TTL; past the
   deadline the title becomes `unavailable` and the requester's `Cancel` runs.

## Requesters

`requester.provider` selects `seerr` (default) or `arr`. Both implement `Request`, `Cancel` and a
`Reachable` probe for the Settings test button.

**Seerr.** `POST /api/v1/request` with `{mediaType, mediaId, seasons}`; `seasons` is required for
series. 201 and 409 both count as success. Seerr has its own approval workflow: the Loomarr service
user must have auto-approve in Seerr, or every approved acquisition stalls in a second queue until its
deadline. `Cancel` is a no-op.

**Sonarr and Radarr.** Movies go to Radarr and series to Sonarr. `Request` resolves the title through
`/api/v3/{movie,series}/lookup` and adds it monitored with a search; 400 or 409 for an already-added
title counts as success. The quality profile and root folder default to the first each service lists,
unless `sonarr.*`/`radarr.*` settings pin them. `Cancel` is a real withdrawal: it deletes the title's
queue record so a given-up download stops.

## Outbound client rules

Every outbound client comes from one HTTP factory with a hard timeout that covers the whole logical
request, including retries and body reads: media server, Seerr and TMDB 10 s, Tunarr 20 s, model calls
120 s.

- At most four attempts, **only for `GET`** (and only when a request body can be replayed). Writes are
  never retried by the client: the idempotent reconcile loops and periodic sweeps own write recovery.
- Retried outcomes: transport errors while the context is live, and HTTP 408, 429, 500, 502, 503 and
  504. A failure while reading a successful body is never retried.
- Backoff is full-jitter exponential from 200 ms, capped at 2 s, and honours a valid `Retry-After`. If
  the server asks for longer than the cap, or the wait does not fit the remaining deadline, the response
  is returned instead of retrying early.
- The discarded body is drained up to a bound before the next attempt, keeping connections reusable.
- Only `GET` follows redirects; any other method returns the redirect to its caller.

A dependency that is down degrades its feature and shows on the setup checklist; it never wedges the
process.

## Tests that pin this

Formerly `design.md` §19.

- **State machine:** every transition and the five invariants.
- **Webhook idempotency and replay:** duplicate and out-of-order events converge.
