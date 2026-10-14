# Pairing &amp; onboarding states — design review candidate

Draft for the maintainer, ahead of any build PR, covering the states
[#1817](https://github.com/loomarr/loomarr/issues/1817) and the roll-up's **row 1 blocker** on this surface call
out: pairing expiry/invalid/pending/success, onboarding's optional steps, permissions, failure/recovery and
long-title states, on both the web-admin and TV/phone sides. Tracking:
[#1817](https://github.com/loomarr/loomarr/issues/1817),
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a review aid for states that are **already
built** (map PR 12, `#1837`) plus the gaps and open questions found while recreating them — it proposes no new
component, route or API, and changes no production code.

Open [the interactive prototype](../prototypes/pairing-onboarding.html) directly in a browser (works offline, no
server calls). Its controls switch surface (Approve a device on web / Code on the device, TV or phone / Setup
wizard), state, frame, the approver's role (admin/member) and a long-names toggle. All fixtures are invented; no
real titles, names or hostnames.

## Why this exists now

Map row 517 ("## Pair a TV") flagged one **Major**: "The TV gets member access" is ambiguous — does the device
act as the person who paired it, or as a generic member (**Q-N3**)? The Decisions table resolved Q-N3 on
2026-09-28 ("acts as the person who approved it") and the build has since gone further: **ADR 0043**
(2026-10-03, `docs/design/decisions/0043-devices-act-with-their-approvers-role.md`) **supersedes** the interim
member cap (#1772) entirely — a device now gets its approver's *real* role, admin included. Pair (#1753), the
device-role cap (#1772) and wizard step gating (#1711) are all merged; #1835 layered the ADR 0043 role change on
top. So the open work left for this draft is not "design the screen" (it's built) — it's walking every state the
brief named against what is actually built, on both the approving side and the device side, and surfacing what
is genuinely undecided rather than guessing it.

## What this draft found, state by state

### Approve a device (web) — `pair-device.tsx`, `routes/pair.tsx`

- **Signed-out (permission denied).** Not an error state by design — `pair.tsx`'s own comment calls it the
  EXPECTED arrival state, because the person is often typing on a phone they haven't signed into yet. The code
  rides through `?code=` (sanitised by `safeUserCode` in `pair.tsx`, never auto-submitted) so returning from
  sign-in lands back here with the code intact.
- **Code entry / typed / checking.** Recreated verbatim: the oversized, monospaced, all-caps field
  (`autoCapitalize="characters"`, `autoComplete="one-time-code"`) sized for reading a code off a TV across the
  room, Add device disabled until non-empty, "Checking…" while `useDevicePairApprove` is pending.
- **Wrong or expired.** The exact built copy ("That code is wrong or has expired…") covers *both* a mistyped
  code and a dead one. This is not a gap — `internal/api/deviceroutes.go`'s `handleDeviceApprove` returns the
  identical 404 for `ErrNotFound` on a missing pairing and on a failed `Approve` call, deliberately: a code
  that reveals "wrong" vs. "expired" vs. "doesn't exist" is an oracle an attacker can grind. Recorded here as a
  resolved question, not an open one.
- **Rate limited (429) and network/server error — a real gap.** `deviceroutes.go` has a per-user+per-IP
  `deviceLimiter` that returns a distinct `429 "Too many pairing codes tried"`, and any network failure or 5xx
  from the mutation is a third distinct cause. But `pair-device.tsx` has exactly one `approve.isError` branch,
  which always renders the "wrong or expired" string — so a rate-limited person is told to recheck a code that
  was never wrong, and a dropped request looks like a bad code instead of something worth retrying. The inputs
  don't decide whether to fix this, so the prototype shows **two labelled alternatives** per state rather than
  picking: **Alt A** — today's single message (as built); **Alt B** — distinguish by cause, with its own retry
  affordance for the network case (retrying a bad code can't help; retrying a dropped request can). Not decided
  here — a build-PR-sized question, since it touches `approve.isError`'s branching, not just copy.
- **Success.** The built card verbatim ("`<name>` is ready… remove it any time from Settings → Access and
  devices"), plus a permission line stating the consequence of ADR 0043 for the current role toggle: admin
  wording ("can approve requests, change settings, reveal secrets… a shared-room TV paired from an admin
  account is an admin") vs. member wording, both pulled from the ADR's own Consequences section rather than
  invented.
- **Long names.** The device name truncates with an ellipsis (`title` attribute carries the full string),
  matching the `truncate` class `paired-devices.tsx`'s `DeviceRow` already applies to the same field. The
  invented long device name is clipped to 64 characters to match `deviceStartInput`'s own
  `maxLength:"64"` (`internal/api/deviceroutes.go`) — the server, not this draft, is the source of that limit.
  The person name has no server-side cap, so the long fixture is a plausible long real name rather than a
  stress string.

### Code on the device (TV / phone) — `pairing-shell.tsx`, `pairing.ts`

- **Loading, awaiting-approval, revoked, failed** are recreated 1:1 from `pairing-shell.tsx`'s own branches:
  the QR/code split screen with the live countdown (`Math.ceil((expiresAtMs - Date.now())/1000)`, from a
  10-minute `DEFAULT_PAIRING_TTL_SECONDS`), "This device was disconnected… Pair it again", and "Couldn't
  connect" with both Try again and Choose another server. **Revoked** is this side's permission-denied
  equivalent: the device is deliberately never told *why* it was cut off (owner revoke, user disabled, or a
  401 from a stale credential all land here identically — `docs/design/auth.md`'s Devices section, confirmed
  against `sessionauth.go`).
- **Expired — a real surprise.** The brief asks for an "expired" state on the device side, matching the web
  side's. There isn't one. `PairingSession.begin()` (`web/packages/core/src/pairing/pairing.ts`) treats a dead
  code from `poll()` (`status: "expired"`) as an ordinary loop `continue`: it silently starts a brand-new
  pairing and emits a fresh `awaiting-approval` with a new code and a reset countdown. No error, no transition
  message — the code on screen just changes. This is a deliberate, good self-healing design (nobody has to
  notice or act), but it does mean someone who wrote the old code down on paper gets no signal it died. Shown
  in the prototype as its own state ("Expired — silently refreshed") with the mechanism explained, not as a
  defect to fix — flagging it for the build PR to decide whether a brief "code refreshed" cue is worth adding.
- **Expiring soon — open, shown as two alternatives.** The real component renders the countdown in the same
  plain metadata text all the way to zero; no urgency styling is wired. The prototype shows **Alt A** (plain,
  as built) and **Alt B** (turns red/bold under 60 seconds) side by side. Not decided — this is the second open
  question carried from the review note at the top of the file.
- **Success (paired).** `PairingShell` calls the caller-supplied `renderPaired(state)` once `status` is
  `"paired"` — each client (Android TV, iPhone, the web pairing page used for a browser-based client) owns that
  screen's actual content. The prototype shows a hand-off marker rather than inventing what any one client
  draws there, since that content is out of this draft's scope.

### Setup wizard — `wizard-shell.tsx`, `wizard/steps/steps.ts`

- **Required vs. skippable, exactly as coded.** `wizardSteps()` marks only `users` `optional: true`; `location`
  is required (`isStepDone` needs a non-empty country) with **no** Skip button and Continue disabled until
  filled. `SKIPPABLE = {library, users}` in `wizard.tsx` is a *second*, UI-local list — `library` has no
  `optional` flag on the step itself but still gets a Skip button, because its gating check (`tunarr_library`)
  can never turn green on an internal-playout install and would otherwise strand the operator. The prototype
  recreates both: a required step (Location, unmet → met) and a skippable step (Library, available → skipped),
  with the skipped marker rendered neutral grey per `wizard-shell.tsx`'s own comment ("skipped reads neutral,
  never red — an optional step you passed on is not a failure").
- **Failure/recovery.** The Connections step's media-server check failing, with the exact mechanism the map's
  critique asked to **keep**: "the live connection check with a fix hint." The hint names the specific problem
  (unreachable address) and a Retest re-runs the check in place rather than dead-ending the operator — Continue
  stays disabled only because `media_server` is in `requiredChecks`, not because of anything the hint implies is
  unfixable.
- **Long names.** `UsersStep` (import media-server users) has no dedicated long-name handling today; the
  prototype's own proposed truncation (ellipsis + `title`) is explicitly labelled as *this draft's* answer, not
  something already built, since inventing silent truncation and presenting it as fact would misrepresent the
  code.
- **Not covered:** the map's own **Minor** finding that step 4 (Connections) scrolls horizontally at 390px
  width — the wizard is desktop-first by design (`wizard-shell.tsx`: "the whole wizard is desktop-first, but
  never broken") and no phone frame exists in the real component, so this draft shows the wizard at desktop
  width only rather than inventing a phone layout the map never asked for.

## Open questions this draft did not settle (show, don't guess)

1. **The web approve page's single error message for three different causes** (bad/expired code, rate limit,
   network/server error) — Alt A (today's one message) vs. Alt B (distinguish by cause with its own retry
   affordance), shown in the *Rate limited* and *Network error* states. A build-PR decision, not a copy fix.
2. **Whether the TV/phone countdown should turn urgent near expiry** — Alt A (plain, as built) vs. Alt B
   (urgent styling under 60s), shown in *Expiring soon*.
3. **Whether the silent code-refresh on the device side needs a transition cue** — raised in *Expired —
   silently refreshed*; not an alternative with two options, just a gap worth a maintainer call.

## Source grounding

- `project/redesign-map-2026-09.md` — `## Pair a TV`, `## Setup wizard` and their critique; Decisions table row
  **N3**.
- `docs/design/decisions/0043-devices-act-with-their-approvers-role.md` (ADR 0043) and
  `docs/design/auth.md`'s Devices section.
- Merged build: `web/apps/web/src/routes/pair.tsx`, `web/apps/web/src/settings/pair-device/pair-device.tsx`,
  `web/apps/web/src/settings/paired-devices/paired-devices.tsx` (#1753); `web/packages/ui/src/pairing-shell/`
  (`pairing-shell.tsx`, `pairing-shell.type.ts`); `web/packages/core/src/pairing/` (`pairing.ts`,
  `pairing.type.ts`); `web/apps/web/src/routes/wizard.tsx`, `web/apps/web/src/components/loomarr/setup/
  wizard-shell/wizard-shell.tsx`, `wizard-shell.type.ts`, `web/apps/web/src/wizard/steps/steps.ts` (#1711).
- Backend: `internal/api/deviceroutes.go` (`handleDeviceStart`'s `maxLength:"64"` on `deviceName`,
  `handleDeviceApprove`'s collapsed 404, the `deviceLimiter`'s 429, `handleDevicePoll`'s 428 vs. 404).
- Generated API models (`web/packages/api/generated/model/`): `deviceDTO.ts`, `deviceApproveOutputBody.ts`,
  `deviceStartOutputBody.ts`, `devicePollOutputBody.ts` — field names and the 64-char device-name cap read
  directly from these rather than guessed.
- Visual tokens: `web/packages/design-system/src/tokens/tokens.ts`, dark theme (canvas `#0B0C0E`, raised
  `#131519`, elevated `#1B1E24`, hairline `#2A2E37`, signal `#FFB020`, success `#3DD68C`, danger `#E85A5F`,
  warning `#F5D90A`).

## Verification

No browser or Playwright install is available on this machine (checked: no `playwright` package in
`web/node_modules`, no `chromium`/`google-chrome`/`chromium-browser` binary on `PATH`) — recorded here rather
than claimed, matching the native-requests draft's own note. What was actually done:

- `node --check` against the prototype's extracted `<script>` body: no syntax errors.
- Manual read-through of the rendered HTML/CSS/JS for every (surface × state) combination against each
  component's real source file, confirming the copy, field behaviour and gating logic above against the actual
  code rather than from memory.
- Every interactive element (`signin`, `submit`, `retry`, `newcode`, `pairagain`, `chooseserver`, `back`,
  `next`, `skip`, `retest`) is wired to a `data-act` handler that only announces the tap in the status line —
  none performs a network call, matches the acceptance criteria's "offline, interactive" requirement.
- Confirmed the long-name fixtures against their real caps: the device name is clipped to 64 characters to
  match `deviceStartInput`'s `maxLength`; the person name is not clipped at the fixture level (no server cap
  exists) and relies on the CSS `truncate` class instead, matching `DeviceRow`'s own approach.

This is not a device test, an accessibility audit, or a production integration check — only a layout/copy/logic
sanity check of the standalone file, cross-referenced against the real source. The acceptance criteria allow a
local Playwright check of the prototype only; this machine cannot run one.

Local reproduction: open `project/prototypes/pairing-onboarding.html` directly in a browser (no server needed —
there are no external resource loads).
