# Consequences of taking over playout (archived)

Archived verbatim from `docs/design.md` §9.1 in #779 (2026-09-27). Not current guidance: the rules
that still bind are in [`docs/design/playout.md`](../../docs/design/playout.md#constraints-that-follow-from-owning-playout).

1. **`ffmpeg` becomes a core runtime dependency** (§14) and ships in the single image (§16). The
   previous "two tags, one binary" split — a 31 MB default plus a 549 MB `loomarr:filler` (retired-ok) — collapses
   into one 549 MB image. An 18× increase in the default download, accepted because a playout-capable
   Loomarr without an encoder is not a coherent artifact.

   ⚠ **"ffmpeg is present" is not one fact — capability is per BUILD, and the build is the only
   honest source.** The image controls its own ffmpeg; a developer running the binary on a host
   does not, and distro/Homebrew bottles differ in which *optional* pieces they carry. The
   detector already takes this seriously for encoders (`listEncoders` asks the binary rather than
   inferring from hardware). **The same rule binds every optional filter, and `drawtext` is one:**
   it needs libfreetype at compile time (plus libharfbuzz on ffmpeg 8), and a build without it
   rejects the filter at graph-init with *"Filter not found"* — the encode exits 8 and the channel
   is dead. Font *discovery* cannot answer this: the card code asked only "is there a font file?",
   which on macOS is yes (Arial ships with the OS) while the Homebrew bottle carries no freetype
   — so every offline card died on a machine that looked correctly provisioned. **The contract is
   that a card degrades to an unlabelled colour field, never to a dead channel**, so text is
   probed like an encoder and an unprobeable ffmpeg resolves to *unlabelled*, never to an assumed
   yes. This is a Linux/macOS parity requirement, not a macOS workaround — a minimal Linux ffmpeg
   fails identically.
2. **`ffprobe` returns.** §16 excluded it on the grounds that *"Loomarr never probes media — Tunarr
   assigns duration during its `local`-source scan."* Once Loomarr owns playout it owns duration, so
   the premise no longer holds. This is the second reversal in this section; both follow from the
   same root cause.
3. **Loomarr publishes its own M3U/XMLTV**, so the §6 Live TV wiring points at Loomarr rather than
   Tunarr for internal channels. `StaleLoomarrListings` currently identifies Loomarr's provider **by
   its Tunarr-shaped path** — retargeting silently breaks stale cleanup unless that identification
   changes with it. *(Both halves are now done: `isLoomarrManagedGuidePath` matches Tunarr's shape
   AND internal playout's, and `LiveTVURLsFor` selects the URL pair from `playout.backend`. Until
   the latter landed the wiring built Tunarr's URLs unconditionally while the backend defaulted to
   `internal`, so the media server was registered against a backend that was not serving those
   channels — the channels appeared in Emby's guide and refused to play, and a `livetv-reconnect`
   "repaired" it by re-registering the same wrong URLs. The URLs resolve **per call**, not at
   construction, so switching backends applies without a restart; and an internal backend with no
   `server.public_url` yields NO urls rather than a relative path, because the media server
   resolves the URL from its own host and would silently point at itself.)*

   ⚠ **Every "what is on now" reader must select on the backend too — the M3U/XMLTV pair was
   not the only place this was wrong.** `GET /v1/channels/now-next` and `…/{id}/upcoming` read
   **Tunarr's** guide, keyed by `TunarrID`, and were wired on `tunarr.url != ""` alone. A channel
   that has been reconciled to Tunarr in the past keeps its `tunarr_id` and Tunarr keeps
   generating listings for it, so after a switch to internal playout the endpoints kept answering
   — from a schedule with its own independent epoch. Observed on the dev install: the guide and
   XMLTV said *The Last Jedi*, `now-next` said *The Rise of Skywalker*, ~30 minutes apart, at the
   same instant. Neither was stale in the caching sense; they were two different schedules.

   **The rule: a reader answers for the backend that is actually streaming that channel, or it
   does not answer for that channel at all.** For internal channels now/next comes from
   `BroadcastsBetween` — the same resolver the encoder and XMLTV already share — so §9.1's
   one-source guarantee covers the card too. Mixed installs resolve **per channel**, via the same
   `policy.playout.backend` precedence `playoutChannels` uses; there is no global switch here
   either. A Tunarr-backed channel with no `tunarr_id` still yields no entry, exactly as before.
4. **Restart is no longer free.** Prior copy promised *"Channels keep playing — Tunarr streams them,
   not Loomarr."* For internal-playout channels a restart **does** interrupt playback, and any
   restart UI must say so rather than inherit the old reassurance.

   ⚠ **The honest copy is PER-BACKEND, not a flat reversal** (recorded during V16, when the
   telemetry made the mechanism concrete). ffmpeg is spawned under one Unix process-group owner
   on the supported server runtime, so a restart kills every helper and
   stream Loomarr is encoding, while Tunarr-backed channels genuinely do keep playing, exactly as
   the old copy said. Since `policy.playout.backend` is per channel, an install can have both at once. The
   restart dialog (V13) therefore needs the live session count, which `GET /v1/playout/sessions`
   now provides: *"3 channels Loomarr is streaming will drop for a few seconds; Tunarr-backed
   channels keep playing."*

   **Tree ownership is one process primitive, not a playout-only convention.** Every item
   encoder, and filler's bounded ffmpeg transcodes, enter the same supervisor;
   cancellation, a natural parent crash, and in-process restart therefore sweep descendants with
   the same Unix-process-group guarantee. A direct `exec.CommandContext` around a
   lifecycle-owned encoder or ingest transcode is a violation because it kills only the immediate
   child.
