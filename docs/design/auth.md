# Authentication and permissions

Formerly `design.md` §11. Loomarr owns identity (decision
[0007](decisions/0007-loomarr-owns-identity.md)): the `users` table is the allowlist, and you can sign
in if and only if you have a row. Roles gate everything that spends resources.

## Identity and credentials

- **Three credential paths land on one identity.** Local users verify against a Loomarr-stored
  Argon2id hash. Imported media-server users verify against Emby or Jellyfin and gain an offline
  Argon2id fallback after their first successful provider login. SSO users verify against an OIDC
  provider. Loomarr stores only non-reversible verifiers. APIs expose credential capabilities, never
  hashes.
- `media_server_linked` records provider linkage independently of the optional `password_hash`.
  Role, quota and disabled state are Loomarr's regardless of path.
- **A username with no row is rejected** as `invalid credentials`, indistinguishable from a wrong
  password, even with valid media-server or SSO credentials. Nothing self-provisions.

**Local login** (`POST /v1/auth/login`) verifies Argon2id in constant time. A legacy bcrypt hash is
accepted read-only and replaced with Argon2id on success; nothing writes bcrypt.

**Imported login** delegates to `POST {LIBRARY_URL}/Users/AuthenticateByName`, discards the provider
token (best-effort logout), confirms the id is allowlisted, refreshes name and disabled state, and
replaces the offline verifier. A provider 401/403 is authoritative; only transport failure, timeout
or 5xx may fall back to the stored verifier. Jellyfin needs `Authorization: MediaBrowser Client=…`
on the login request itself, Emby needs `X-Emby-Authorization: MediaBrowser …`; the `DeviceId` is
stable per install.

**Sessions.** Every path issues Loomarr's own session: an opaque 256-bit token in an HTTP-only
`SameSite=Strict` cookie, SHA-256-hashed at rest and resolved on every request. Sessions are listable
and revocable (`GET /v1/users/{id}/sessions`, `DELETE /v1/sessions/{hash}`); disabling a user ends
them at once; `SESSION_TTL` slides. `cookie.secure=auto|always|never`. Mutating routes also require
`X-Loomarr-Csrf: 1`. Login is rate-limited by client IP. Forwarding headers (`X-Forwarded-For`,
`X-Forwarded-Proto`) are trusted only when `security.trust_proxy=true`; otherwise the socket peer is
used, so a direct client cannot forge its way past the throttle or downgrade its cookie.

**Machine access.** The generated `API_TOKEN` authenticates scripts through `Authorization: Bearer`
and is the break-glass admin when no user can sign in.

**Dev login** (`POST /v1/auth/dev-login`) exists only while the server environment sets
`LOOMARR_DEV_LOGIN=1`; otherwise the route is not registered and returns 404. It is a server
variable, never a build flag, so a shipped bundle cannot carry it. It signs in as the lowest-id
existing admin, never creates or promotes a user, refuses when no admin exists, and boot warns on
every start while it is on. `make bootstrap` in a secondary worktree provisions that worktree's own
database; the primary is untouched.

## Bootstrap

First run creates a local admin with zero media-server configuration: `POST /v1/setup/bootstrap`
succeeds exactly once, only while no admin exists, and a second call gets 409. The wizard drives it
first.

## Import and sync

`GET /v1/users/candidates` lists media-server users with an `imported` flag; `POST /v1/users/import`
upserts an explicit set as allowlisted rows, copying the source's admin status on a new import.
`POST /v1/users/sync` (and the periodic sync) refreshes only already-imported users' names and
disabled state; sync never adds anyone. An unconfigured library connection returns the supported
unconfigured response.

## Invitations and recovery

An admin may admit someone without choosing their password. The **Invitation** reserves one local
username or one Library account id plus the chosen role (member by default) and stays outside the
allowlist until redeemed; creating one never creates an active row. Direct create and import remain
available.

- **Lifecycle:** `pending → redeemed`, with `expired` (seven days) and `revoked` as terminal exits.
  Delivery outcomes are not Invitation states; the People UI composes them. Redemption is one
  transaction that claims the Invitation, creates the row, consumes the grant and invalidates its
  siblings, so concurrent submissions produce one account.
- **Grants** are random 256-bit single-use bearer capabilities, stored only as SHA-256 and kept out of
  logs, metrics, diagnostics, error bodies and browser storage. The join page moves the bearer out of
  the URL at once and requires explicit consent; opening or scanning never redeems.
- A local Invitation collects a new password. An imported one authenticates against the Library and
  must match the pinned account id; a provider rejection is final.
- **Contact addresses** are not identity: never a login, never provisioning, never a role. Addresses
  are normalised (one RFC 5322 mailbox, case-folded) and verified only by redeeming a grant Loomarr
  actually emailed to that address.
- **Password recovery** reuses the grant machinery. The request response is identical for every
  account; only an enabled local user with a verified contact gets mail. Grants expire in 30 minutes,
  and a reset revokes every session and outstanding grant in one transaction. Imported passwords
  never enter this flow.
- **Links** use `access.public_url`, the recipient-reachable origin, never `Host` or forwarded
  headers. It is distinct from `server.public_url` (the machine-client address). With it unset,
  email is suppressed and no shareable grant is minted.
- The QR presentation reuses the `QrCode` primitive only; Invitation and device pairing stay separate
  protocols.

## Notifications

Notification delivery is one channel-neutral module. Callers submit a typed **Notification intent**
(a recipient reference and bounded data); the module owns routing, durable work, idempotency,
rendering, retries, retention, and one adapter per **Delivery means**.

- **Two policy classes.** Mandatory account delivery (`account_invitation`,
  `local_password_recovery`) is email-only and ignores preferences. Configurable product delivery
  (`proposal_submitted`, `proposal_approved`, `proposal_declined`, `acquisition_available`,
  `acquisition_gave_up`, `channel_live`, `channel_degraded`) goes to one person, current approvers or
  operators.
- **Means** are a closed vocabulary (`email`, `webhook`, `discord`, `ntfy`, `gotify`, `apprise`,
  `pushover`, `telegram`, `mattermost`, `matrix`, `web_push`, `mqtt`, `slack`); an unimplemented one
  ends as `means_unavailable`. Adding an adapter never changes a domain interface. New installs and
  upgrades start with no product routes.
- **Publication is best-effort** after the domain transition and never rolls it back. Approval records
  requester provenance for each approved Title and Channel in the same transaction, which is the
  durable backstop for later notices.
- **Destinations** own one means, a label, enabled state, compatible topics and write-only
  credentials. Installation destinations are admin-owned and address approvers or operators; person
  destinations are a person's verified email or their Web Push subscriptions, re-authorised at event
  time. Reads never return credentials, URLs with bearers, push keys or addresses. Credentials are
  sealed as one record-bound envelope and re-sealed on key rotation.
- **Setup** is one provider-led form per means in Settings → Notifications, driven by a server-owned
  provider definition that also decides which fields are secret. MQTT requires certificate
  verification with no bypass. Browser Push asks for permission only on an explicit user gesture;
  a 404/410 from the push service disables the destination. Legacy `notifications.email.*` and
  `notifications.smtp.*` settings were migrated once into an SMTP destination and are never read again.
- **Attempts** record ids, means, safe labels, status (`queued | sending | delivered | failed |
  suppressed`) and a scrubbed error class, never content or credentials. Email grants are minted only
  when an attempt starts. A failure that may have followed SMTP acceptance is ambiguous: the grant
  stays valid and automatic retry stops, which favours a duplicate notice over a duplicate account.
- **Retries** are fixed policy: five attempts (about 1 min, 5 min, 30 min, 2 h, with jitter) for
  failures known to precede acceptance; `Retry-After` may lengthen a wait up to two hours. Terminal
  intents, attempts, Invitations and grants are purged after 30 days.

## Devices

**Paired clients** hold a durable, member-scoped bearer credential from the device-code flow, kept in
the platform secure store. **Disconnect this device** calls `DELETE /v1/auth/device` with that
credential and may revoke only that device; a session or `API_TOKEN` cannot substitute. The client
clears its credential only after the server confirms or a 401 proves it is dead.

**Playout devices.** A television cannot hold a session, so tuner, guide and segment routes accept a
`playout_token` instead. It is the only path that does not resolve to a `users` row: read-only by
construction, playout-scoped, never a user, redacted everywhere, and rotated only behind a typed
confirmation because rotation breaks the media server's wiring. The in-app player is a person and
gets a derived, expiring signed URL for one channel, never the token
([`playout.md`](playout.md#admission)). `API_TOKEN` is full authority; `playout_token` is almost
none. They must not be conflated.

## Roles and quotas

- **`admin`**: approve, destructive channel actions, users, settings, jobs, filler.
- **`member`**: browse, run suggestion jobs, submit Proposals.
- **Quotas:** a per-user pending-acquisition cap (0 means `suggest.max_acquisitions`) and an optional
  `auto_approve` grant. Pending acquisitions are attributed through the requester's approved
  Proposals, deduplicated by title key, until the title is terminal. The cap binds only automatic
  approval; manual and automatic approval run the same code path
  ([`proposal-workflow.md`](proposal-workflow.md#approval)).

## Route authorization

Every route declares its role at registration and middleware enforces it (decision
[0012](decisions/0012-routes-declare-their-role.md)): `RoleAdmin`, `RoleMember`, or an explicit
`RolePublic` for the pre-authentication routes (login, logout, bootstrap, setup state, version). **A
route with no declared role is refused.** A test enumerates the registry and fails on any operation
without a role.

Plain-mux routes (SSE, backup download, icons, playout, SSO redirects) bypass Huma middleware and use
one shared guard that fails closed: no authorizer means denied.

## SSO

OIDC is a credential path, not a provisioning path (decision [0011](decisions/0011-sso-is-a-credential-path.md)).

- An SSO identity without a row is rejected; there is no auto-create and no group-to-role mapping.
  Loomarr's own sign-in always works alongside it.
- **OIDC only, not forward-auth:** header trust is only as strong as the network wiring, and a
  Loomarr reachable beside its proxy would accept any `Remote-User`.
- A login matches the `preferred_username` claim (else `email`) against `users.name`. Claims come
  from the userinfo endpoint, with its `sub` cross-checked against the token's. The issuer is
  matched exactly, trimming only whitespace.
- The callback is bound to the browser that started it: `start` sets a short-lived state cookie with
  `SameSite=Lax` (Strict would drop it on the provider's cross-site redirect), and `callback` refuses
  without a match. PKCE S256 is sent too.
- Redirect targets (`next`, and the SPA's `redirect`) are parsed, not prefix-matched, and re-validated
  where each navigation happens, because browsers treat `\` as `/`. `safeReturnPath` and the
  frontend's `safeRedirectPath` implement the same rule.
- `make test-sso` drives real Authelia and Authentik instances; each found a defect the other could
  not. Authentik needs a signing key on the provider, and Authelia's cookie domain must match the URL
  Loomarr is reached at.

**Only a 401 signs a user out.** The web auth guard rethrows any other failure of the identity
query, so a restart or proxy error never looks like a logout.

## Tests that pin this

Formerly `design.md` §19.

- **Auth and roles:** bootstrap creates the first local admin exactly once (a second call 409s); a
  local user logs in against Argon2id (a verified legacy bcrypt row upgrades in the same login); an
  imported media-server user prefers provider auth and falls back only while the provider is
  unavailable, never after a rejection; an un-imported media-server user or an SSO identity with no
  allowlist row is rejected even with valid credentials, and no login path creates a row; import is
  admin-only and sync never adds; `member` gets 403 on approve, admin routes and `POST /v1/titles`;
  disabling a user revokes their sessions; `API_TOKEN` grants break-glass admin; plaintext
  passwords and media-server tokens are never persisted.
- **Invitation and contact store conformance** runs one suite over SQLite and Postgres: normalized
  contact uniqueness, reserved identity collisions, lifecycle transitions, regeneration and
  revocation, expiry, verified-contact replacement, grant hashes never yielding a bearer, and two
  concurrent redemptions producing exactly one user and session.
- **Access security negatives:** members get 403 on every Invitation, contact and delivery admin
  mutation; anonymous callers cannot inspect reservations or delivery; disabled users lose
  sessions; unlisted library accounts look like bad credentials; imported recovery never sends or
  resets; public recovery does not enumerate eligibility; provider rejection during imported
  redemption never falls back; and no password, token, SMTP credential or plaintext grant appears
  in fixtures, logs, metrics, diagnostics, activity, problem bodies, browser storage or examples.
- **Notification certification:** a deterministic delivery adapter pins intent idempotency, retry
  timing, suppression, retention and ambiguous acceptance without a network. SMTP runs against an
  in-process server: unauthenticated and authenticated submission, STARTTLS, implicit TLS,
  certificate rejection, text plus HTML MIME, permanent rejection, transient retry, disconnect after
  `DATA`, cancellation and redaction.
- **Contract and frontend:** OpenAPI and orval types cover every route and lifecycle shape. Stories
  use synthetic grants and cover email configured or not, delivery states, local and imported
  Invitations, invalid grants, recovery and QR/copy at desktop and mobile widths. Vitest pins URL
  cleanup and zero browser persistence; Playwright pins explicit-consent redemption, focus return,
  the shared live region, forced colors, axe and visual baselines.
