// Package playout is Loomarr's own streaming engine (design §9.1): it turns a channel's
// computed lineup into a continuous MPEG-TS a media server can tune, without Tunarr.
//
// # The mechanism
//
// The hard part of live TV is not encoding, it is CONTINUITY: one unbroken byte stream whose
// content changes at programme boundaries. Splicing encoders together is the obvious approach
// and it is a tar pit — timestamps, keyframes, and stream parameters all have to be reconciled
// across the join.
//
// This package does not splice. Following Tunarr's design (docs/engineering/archive/playout-prior-art.md
// §1), ONE long-lived parent ffmpeg runs `-c copy` over a two-line HTTP ffconcat playlist whose
// entries BOTH resolve to "what is on now". Each time the concat demuxer opens an entry it asks
// the server that question, receives one finite programme, plays it to EOF, and advances to the
// other entry — which asks again. The demuxer's EOF-and-advance IS the programme boundary. There
// is no splicing code in this package because there is no splice.
//
//	parent ffmpeg ──> GET /playout/playlist/{ch}   (two lines, both "what's on now")
//	                    └─> GET /playout/program/{ch}  ──> one finite encoded programme
//	                    └─> GET /playout/program/{ch}  ──> the next one, asked at EOF
//
// # The invariants
//
// These are the properties the rest of the system relies on. Each one has a test, and each one
// has broken a live channel at least once when it did not.
//
//   - ONE SOURCE OF TRUTH FOR "WHAT IS ON". Reconciliation persists the accepted Desired cycle;
//     AiringAt and the current BroadcastsBetween segment walk the same cycle arithmetic over that
//     snapshot. CyclePreview forecasts future windows and unsaved edits, but is never called by the
//     encoder at a programme boundary: its mutable airing-history inputs could otherwise reorder
//     the deck while it is playing.
//
//   - A CHANNEL IS A WALL CLOCK, NOT A PLAYLIST. Tuning in 40 minutes into a 60-minute film
//     lands 40 minutes in, for every viewer simultaneously. That is what `epoch` anchors: a
//     persisted first-live origin that survives restarts and reconciles. It is stamped once,
//     rather than recomputed from query time, process start, or Channel.UpdatedAt.
//
//   - ONE ENCODER PER CHANNEL, N REFCOUNTED VIEWERS. A second viewer joins the existing stream
//     rather than starting a second encode. Admission is bounded (AtCapacity); viewers are
//     never EVICTED to make room, which was viewra's mistake — dropping a watching household to
//     admit a new one trades a working stream for a broken one.
//
//   - A SLOW VIEWER IS DROPPED, NOT BUFFERED. Note this INVERTS internal/events, which drops the
//     MESSAGE and keeps the subscriber: an SSE client can refetch state on reconnect, so a gap
//     is recoverable. A byte stream with a hole in it is corrupt, not stale — so here the
//     viewer goes and the stream stays clean.
//
//   - NOMINAL TIME IS NEVER AIRTIME. A pending acquisition has no known duration. Giving it one
//     inside the cycle arithmetic would grow the cycle, shift every later programme, and make
//     the encoder air silence for an invented length. Display width for such a slot is a
//     PROJECTION concern (BroadcastsWithPending), never a timeline one (slotDuration).
//
// # What lives here, and what deliberately does not
//
// This package is the MECHANISM: encoder capability probing, argument construction, the quality
// ladder, session/viewer bookkeeping, cycle arithmetic, and XMLTV rendering. It knows nothing
// about stores, media servers, settings, or HTTP handlers.
//
// The WIRING lives in internal/app (playoutadapter.go) and internal/api. That split is why this
// package can be tested without a database or a network, and it is worth preserving: the moment
// playout can reach a store, "what is on now" grows a second answer.
//
// # Encoding
//
// Detect() TRIAL-ENCODES each candidate rather than trusting `ffmpeg -encoders`, which happily
// lists encoders the hardware cannot run — the trap that took a live channel down when a host
// advertised h264_vulkan and the container had no /dev/dri. Nine encoder families are supported
// with per-family argument vocabularies; software is always the floor, never a failure.
//
// See PROGRESS.md ("Playout: five traps that each cost a live channel") for the hardware-encode
// findings — four of them had green tests over them the whole time.
package playout
