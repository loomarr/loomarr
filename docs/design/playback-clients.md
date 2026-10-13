# Playback clients

Formerly `design.md` §9.1's tuning (V57), Android TV and pause (V60) sections. Clients play the
signed HLS that [`playout.md`](playout.md) serves. Distribution of the Android TV app and the layered
browser and runtime certification are release and testing mechanics, tracked on #1572.

## Tuning

Tuning is a latest-request-wins state machine owned by one **Tuner controller**, not by route
mounting (decision [0021](decisions/0021-tuning-state-machine.md)). The URL follows the selected
Channel for refresh and deep links; a route transition is an output of a tune, never what sequences
playback.

- **One surfable catalog**, ordered by Channel number with wraparound, carrying the now/next row.
  Whether a Channel plays in-app is server-derived from its effective backend. Paused, detached,
  empty and Tunarr-backed Channels stay in the Guide but are not surfable.
- **Attempt ids.** Every request gets a monotonically increasing id. A newer request aborts the
  older play-URL mint, manifest fetch and HLS attachment, and callbacks check their id before
  changing state, so a slow old response never replaces the newest Channel.
- **One player.** At most one `<video>` element and one decoder exist; prefetch never creates a
  hidden player, `MediaSource` or decoder. The contract is platform-neutral: each platform adapter
  binds its native player behind the same catalog, attempt and telemetry vocabulary.
- **Controls.** Channel Up/Down are visible controls; `ArrowUp`/`ArrowDown`, `PageUp`/`PageDown` and
  `ChannelUp`/`ChannelDown` invoke them when focus is not in a control that owns those keys. The OSD
  switches at once to the target and `Tuning…` and stays over the held frame until the replacement
  decodes. A failed tune keeps the target visible with a retry. Digits tune a channel by number
  (up to three, 1.2 s to finish, `Enter` tunes at once), the same entry the TV remote uses; on the
  web, `G` opens the channels drawer. Both go through the same latest-request-wins tune. A settled
  tune (its first frame) records itself in the viewer's recents; a warm or a channel surfed past
  never does.
- **The playhead describes the decoded frame,** from the program-date-time mapping of the frame on
  screen, not `Date.now()`. A stalled frame freezes programme context instead of claiming a break
  has begun.
- **Warming is still-first and uses only spare room.** The client keeps signed play URLs for the
  previous and next Channels and prefetches their stills (`GET /playout/still/{id}`); a tune paints
  that still as the poster until the first decoded frame. Warming starts only after the current
  Channel's first frame, and a new tune aborts it. Its `mode=warm` master starts the neighbour's
  baseline packager under speculative admission: into spare room only, and a viewer's tune evicts it
  first (#1780). For a viewer whose client took a premium playlist in the last 15 minutes, the warm
  starts the neighbour's premium too, after its baseline. A premium warm evicts nothing, and any
  baseline, even another warm, may reclaim its room (#1037). Every admit, refuse and evict decision is
  logged at INFO as `packager hls: admission`.
  The warmer follows the master's variant playlists with `mode=warm`, then drains each available
  rendition's active init and newest media fragment. A warm variant read only retains an already
  admitted packager; it never starts one, promotes speculative work, learns a premium preference,
  or records household viewing. Premium reads require the viewer's learned premium preference.
  A missing rendition or asset is an unsuccessful warm. The real tune reuses the original signed
  master without `mode`; native clients skip it only when it offered exactly one rendition.
- **hls.js settings** match the packager: `lowLatencyMode: false`, `liveSyncDuration: 6` (its
  `HOLD-BACK`), `startFragPrefetch: true`. The TV player sets `minBufferForPlayback: 1`.
- **Web source swaps.** Safari-family WebKit uses native HLS when it exposes it; Chromium and Firefox
  use hls.js (a MIME answer alone is not trusted). The Web adapter keeps at most two source-scoped
  hls.js controllers, one active and one never-used standby, over the single element. A replacement
  either transfers compatible SourceBuffers through hls.js's cross-controller handoff (Chromium and
  Firefox, within a 100 ms seek bound) or attaches a fresh `MediaSource` to the same element
  (WebKit, ended sources, missed bounds). The held poster covers either path, the outgoing blob URL is
  always revoked, and no init or media fragment is loaded into a detached controller.
  hls.js's optional interstitial controller is omitted: commercials are already encoded into the
  channel timeline, and every retired controller must release its video listeners after transfer.

The controller publishes User Timing measures per attempt (OSD paint, manifest, first decoded frame)
with no Channel ids or titles in the names. The #1512 exit criteria on a 100-Channel catalog: OSD p95
under 100 ms; warm change p95 under 600 ms; cold tune to first frame p50 ≤ 1.0 s and p95 ≤ 1.5 s for
SDR (≤ 2.5 s for 4K HDR); a burst of twenty requests plays only the last target and starts no
adjacent encoder. Not all are measured yet; the Shield run and 24 h soak are #1037.

## Favourites and recents

The guide's **All · Favourites · Recent** filters, the surf rail's groups and the watch console's star
read one per-person contract (#1666). The lists belong to the person, not the device: a paired TV acts
as the person who paired it, so a Channel starred on the TV shows on their phone. A break-glass
`API_TOKEN` caller has no person and gets 401.

- `GET /v1/me/channels` returns both lists in one read: favourites in the order they were starred,
  and up to 20 recents, newest first. The filter counts are the list lengths.
- `PUT` / `DELETE /v1/me/favourites/{channelId}` star and unstar, idempotently, and return both lists.
- `PUT /v1/me/recent-channels/{channelId}` records a tune. A client calls it once a tune settles
  (first decoded frame), never for warming: minting a play URL is not a tune, because the warmer
  mints them for the neighbouring Channels. A late report never moves a Channel backwards.
- Both lists die with their person or their Channel.

## Household viewing

Home's **Watching now** reads `GET /v1/household/viewing` (#1662). Who is watching is the server's
own observation, not a client report: a live player re-reads its media playlist every segment, so a
device is watching a Channel while its polls continue (`internal/viewing`).

- **Attribution.** A play URL carries a signed `viewer` tag naming the person and device it was
  minted for: a paired device by its name, a browser by its family and system ("Firefox on
  macOS"). The master copies it onto the media-playlist URLs, so no client change is needed. It
  grants nothing (`sig` alone authorizes the stream), but it is HMAC-signed and bound to the
  Channel, so nobody can make the household believe someone else is watching.
- **What counts.** Two polls within 30 s make a viewing; the warmer's single fetch of a neighbour
  does not. A device watches one Channel at a time. Thirty seconds without a poll ends a viewing.
  Media-server Live TV viewing carries no person and is not counted.
- **Who sees what** (#1659 H2, enforced by the server): an admin gets every viewing by name
  (`scope: household`); a member gets the per-Channel counts plus only their own viewings
  (`scope: self`). Each person also gets `continueWatching`, their latest settled tune from
  [Favourites and recents](#favourites-and-recents), which is also what Watch opens.
- The tracker is in memory: one replica is the supported topology, and a restart loses nothing
  a poll interval doesn't rebuild.

## Pause

Pause is shared time-shift, not a private playback stack (decision
[0022](decisions/0022-pause-is-shared-time-shift.md)).

- Pausing freezes the viewer's broadcast position; resume continues from it while it stays inside
  the fifteen-minute `DVRHorizon`. The player shows how far it trails live and one **Go Live**
  action. An expired position returns to the live edge and says so. Tuning always joins live.
- The Web adapter records the media time and the viewer's wall-clock time, pauses the one element
  without stopping the shared session, and resumes by seeking when the position is still seekable.
  hls.js back-buffer and latency correction are set beyond the horizon. A tune's replacement
  callbacks never resume a deliberately paused viewer.
- Programme time and the mini-guide follow the viewer's clock, derived from
  `EXT-X-PROGRAM-DATE-TIME` for the frame on screen (hls.js `playingDate`, native HLS timeline, or
  Media3's Window clock minus `currentLiveOffset`).
- The history is the packager's one rolling window on disk; a reader behind it gets
  `ErrSegmentGone`. Pausing ten viewers creates no extra packagers. The Watch timeline asks for the
  same fifteen minutes behind live plus its three-hour future. `DVRHorizon` is one exported server
  constant, not a setting.

## Android TV

The paired Android TV client is a watching-first remote surface (decision
[0023](decisions/0023-android-tv-watching-first.md)). It is an adapter over the same Channel, Guide,
device-profile and signed-HLS contracts as Web, acting with the role of the person who paired it
([0043](decisions/0043-devices-act-with-their-approvers-role.md)); it has no admin screens, no
media-server token and no second playback API. It installs as a Leanback launcher app and never edits
the user's home-screen ordering.

- **Watching** is full-screen video. Up/down and Channel keys tune adjacent Channels, number keys
  enter a Channel, OK opens Guide, Left (or Menu) opens Surf, Back returns to the launcher. Channel
  identity and the programme bar appear together and clear after five seconds. During filler the
  label is `Commercials · <clip title>` when the Guide supplies one; content hashes and paths are
  never shown. Nothing the server did not supply (codec, captions, resolution) is invented.
- **Surf** overlays the still-mounted player: the person's favourites and recents (see
  [Favourites and recents](#favourites-and-recents)), then every playable Channel. Its footer shows
  client and server versions.
- **Guide** is a Channel-by-time grid opening on two hours with thirty minutes of lookback. Clocks use
  the time zone `GET /v1/guide` echoes, as 12-hour AM/PM. Up from the first row enters the filter
  row; disabled filters are skipped. The focused programme card shows artwork, titles, time, and
  metadata when present.
- **Focus** is explicit and clamped on every surface. "Now" comes from the server clock, never the
  TV's RTC: the play-URL response's `serverTimeMs`, advanced by local elapsed time.
- **Catalog.** Watching, Surf and Guide share one playable-Channel catalog. `/v1/events` `channel`
  frames only invalidate; `GET /v1/channels` is the authority, re-read on connect, reconnect,
  foreground, coalesced bursts and a five-minute safety interval. A failed read keeps the last
  catalog. The tuned Channel survives by id; if it disappears, the first playable one is tuned.
- **Layout** is authored for the 960 × 540 dp canvas and rendered at 1080p or 4K densities; the Media3
  `SurfaceView` is full-screen, so UI density never caps the stream.

**Finding a server.** An unpaired TV normally never asks for a URL. Loomarr advertises
`_loomarr._tcp.local.` over DNS-SD. Because Docker bridge networking does not carry multicast DNS,
the container install also answers UDP port `51029`: the exact datagram `LOOMARR_DISCOVER/1` gets a
unicast JSON reply (`protocol: 1`, instance id, name, `server.public_url`). The client broadcasts and
also sweeps at most its own `/24`, twice, 5 ms apart per packet. Neither transport carries a
credential or pairing state; the responder stays silent while `server.public_url` is empty, and a
discovery failure never blocks readiness. The client browses only while the connection screen is in
the foreground, for at most thirty seconds per visit, then leaves manual entry. Discovery grants no
trust: the viewer picks a server and completes the revocable device-code pairing.

**Changing servers.** A catalog failure offers **Retry** and **Change server**; the saved credential
stays until a new pairing atomically replaces it. **Disconnect device** revokes remotely first; if
the server is unreachable, **Forget locally** is a separate, explicit action that says the old server
may still list the device. A 401 clears the credential automatically.

**Acceptance.** An Android UI change needs, in addition to screenshots, a hands-on run of the current
APK in a windowed, centred API-30 TV emulator with the real remote keys on the touched path.

## iPhone and native Requests (N2)

The paired iPhone is a viewer (Watching, Guide, Surf as on Android TV) with one addition the TV does
not have: **channel Requests** (maintainer decision N2 of #1659, 2026-10-03). Native has no Home, so
Requests is where a person asks for a channel and follows it. It is a member-scoped adapter over the
routes Web already uses; no native-only route exists and no authority is added.

- **Two ways to ask, as Web.** *Channel ideas* are the server's cards (`GET /v1/discovery/ideas`);
  **Request channel** is `POST /v1/discovery/ideas/{ideaId}/request` and works with AI off. *Describe a
  channel* is one typed brief (`POST /v1/proposals`) with Web's templates. Both end in the same place:
  a Proposal Job that waits for an admin unless the requester's `auto_approve` grant applies.
- **AI off is Web's behaviour, not a native invention.** When the server answers
  `feature_not_configured` or `grounding_not_configured` the brief is kept and the screen says an
  administrator has to finish AI or TMDB setup. There is no manual lineup on the phone; ideas stay
  available. A member is never told to change an admin-only setting.
- **The list is Web's.** Requests are the caller's own Proposal Jobs (`GET /v1/proposal-jobs?mine=true`),
  grouped as Web groups them: **Needs you** (requests that couldn't be built; for an admin also the
  proposals waiting on a decision), **In progress**, **Done**. The status line comes from one pure
  function over the Journey and its titles' acquisition states, so the badge, the tab and the detail
  cannot disagree. The server's `actions` and `recoveryAction` decide what may be offered; the client
  never reconstructs them.
- **Admin review on an admin-paired phone.** A device acts as the person who paired it. Once the server
  lets an admin-paired device act as admin (a separate change), the phone offers **Approve**, **Deny**,
  **Edit** and bulk approve, gated on the role in `GET /v1/auth/me`, with Web's consequence copy
  (approving creates the channel and starts the downloads; denying keeps the reason with the
  request). Until then `/v1/auth/me` reports `member` and the controls do not appear. **Edit** is
  Web's edit-at-the-gate: the admin unticks titles and adds a note, and both ride the one
  `POST /v1/proposals/{id}/approve` body (`drop`, `note`). Adding a title needs search and stays on Web.
- **Touch and orientation.** Portrait only, touch density, every target at least 44 pt. Live updates
  poll (two seconds while a request is generating); `/v1/events` is only a Web invalidation trigger.
- **Tab position (approved mock, #1840).** Requests is a fourth tab between Guide and Surf (Watching ·
  Guide · Requests · Surf) with Web's count badge: an admin's pending decisions plus anyone's failed
  requests. "Request a channel" is an icon button in the Requests app bar on both phones; Android has no
  floating action button.
- **Admin review at phone width (approved mock).** Needs you shows two independently selectable groups,
  **Channel requests** and **Filler downloads**, each with its own Select and bulk approve and no
  select-all across them, then **Couldn't be built**. Deny, Edit and bulk approve use a bottom sheet; a
  bulk result lists each item's outcome in the `BulkApproveResult` shape. Channel requests bulk-approve
  through `POST /v1/proposals/approve`. Filler downloads have no bulk endpoint yet, so the client
  approves each pull with `POST /v1/filler/pulls/{id}/approve`, one at a time, behind one method that
  returns the same `{id, ok, error}` results; a bulk endpoint replaces that one method. A filler
  download is **dismissed**, not denied, and has no note because it has no requester.
