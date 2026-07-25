package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
)

// The adapter that makes internal playout work (§9.1) — where three separately-verified pieces
// finally compose:
//
//	channels.CyclePreview  → what this channel airs (the SAME call reconcile and the UI use)
//	playout.AiringAt       → which program that puts on right now, and at what offset
//	library.StreamURL      → the URL ffmpeg can actually read
//
// It lives in internal/app rather than internal/playout on purpose. playout is the mechanism
// (encoders, args, sessions) and must not know about stores, media servers, or settings; this is
// the wiring, and wiring belongs in the composition root.

// cyclePreviewer is the scheduling surface the resolver needs — satisfied by *channels.Engine.
//
// Narrowed to the one method deliberately: the resolver must not be able to reconcile, push to
// Tunarr, or mutate anything. Playout is a READ of the schedule, and a narrow interface makes
// that structural rather than a rule someone has to remember.
type cyclePreviewer interface {
	CyclePreview(ctx context.Context, channelID string, at time.Time) (
		resolvedAt time.Time, slots []schedule.Slot, active schedule.ActiveRuleAttribution,
		window time.Duration, err error)
}

// playoutResolver answers "what is airing now, and where does ffmpeg read it from".
type playoutResolver struct {
	engine cyclePreviewer
	lib    *library.Client
	now    func() time.Time

	// tier / encoder / capacity are read live so an operator's Settings change applies to the
	// NEXT program rather than requiring a restart. Each program is a fresh child process, so
	// "the next program" is at most one program away — which makes hot-apply genuinely cheap
	// here in a way it would not be for one long-lived encode.
	tier     func() string
	encoder  func() string
	capacity func() int
	// activeChannels is how many channels are encoding right now, for the load-aware quality
	// ladder. A FUNC because the session manager and this resolver need each other: the manager
	// spawns encodes that ask the resolver for a profile, and the profile depends on how many
	// the manager is running. A func breaks the cycle that a struct field could not.
	activeChannels func() int
}

// AiringNow resolves the channel's current program and its ffmpeg input URL.
//
// It asks the SAME CyclePreview the reconciler and the UI's cycle preview call, which is the
// whole point: what plays cannot drift from what the preview promised. A private "what should
// playout air" path would be the §10 shared-assembler mistake in a new place — two answers to one
// question, guaranteed to disagree eventually.
func (r *playoutResolver) AiringNow(ctx context.Context, channelID string) (playout.Airing, string, error) {
	now := r.now()

	// `at` is `now`, not zero: CyclePreview treats a zero time as "now" via its own injected
	// clock, and passing our clock explicitly keeps this resolvable in tests without reaching
	// into the engine's.
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, now)
	if err != nil {
		return playout.Airing{}, "", err
	}

	airing := playout.AiringAt(slots, playoutEpoch(channelID), now)
	if !airing.Playable() {
		// Not an error: an empty lineup, or one where nothing has landed yet, is a real state.
		// The handler renders it as the offline card.
		return airing, "", nil
	}

	url := r.lib.StreamURL(airing.LibraryItemID)
	if url == "" {
		// The item id is real but the media server is unconfigured, so there is nothing to
		// read. Reporting it as "nothing airing" rather than an error means the channel shows
		// the offline card instead of failing to tune — the same outcome the viewer would get
		// from an error, minus the retry storm.
		return playout.Airing{Kind: schedule.SlotFlex}, "", nil
	}
	return airing, url, nil
}

// Profile is the encode profile for the next program, resolved against live load.
//
// Called once per program (each child is a fresh process), so the ladder adapts as channels come
// and go: the first channel on an idle box gets the top rung, and a fifth channel starting up
// steps everyone down as their next program begins. That is the "best picture the hardware
// sustains, then adapt" policy §9.1 states, and it is only implementable because the child
// processes are short-lived.
func (r *playoutResolver) Profile(_ context.Context) playout.Profile {
	enc := playout.Encoder(r.encoder())
	if enc == "" {
		// No operator override: the capability prober's choice was stored at wizard time. An
		// empty setting here means "pick for me", and software is the honest fallback — it is
		// the one encoder that is always present, and a wrong hardware guess fails at init
		// rather than degrading.
		enc = playout.EncoderSoftware
	}
	return playout.Resolve(playout.TierFor(r.tier()), enc, r.capacity(), r.activeChannels())
}

// playoutEpoch anchors a channel's cycle on the wall clock.
//
// The anchor has to be STABLE, and the obvious candidates are all wrong:
//
//   - `time.Now()` is not an anchor, it is the query.
//   - Process start would make every channel jump back to its first program on restart.
//   - `Channel.UpdatedAt` is tempting — it is stored, and it moves when the lineup moves — but
//     it is stamped on EVERY write including a routine reconcile sweep, so a background job
//     would re-anchor a channel mid-program and the viewer would see it jump.
//
// So: a fixed reference instant plus a per-channel offset derived from the channel id, the same
// deterministic-without-storage trick channels.PodSeed uses. Two channels created together
// therefore do not march in lockstep, the anchor survives restarts and reconciles, and nothing
// needs to be persisted or migrated.
func playoutEpoch(channelID string) time.Time {
	// Fixed, not computed: any drift in this value is a channel jumping. In the past so
	// `now - epoch` is positive for every real clock.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	offset := channelOffset(channelID) % int64(24*time.Hour)
	return base.Add(time.Duration(offset))
}

// channelOffset hashes a channel id to a stable non-negative number (FNV-1a, as PodSeed does).
func channelOffset(channelID string) int64 {
	var h uint64 = 14695981039346656037
	for _, b := range []byte(channelID) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	// Mask the sign bit rather than negating: negating math.MinInt64 overflows back to itself,
	// which would make one channel-id-in-2^64 produce a negative offset and an epoch in the
	// future — and a future epoch clamps to offset 0, silently pinning that channel to its
	// first program forever.
	return int64(h & 0x7fffffffffffffff)
}

// playoutSpawner builds the SESSION encoder for a channel: the long-lived parent that reads the
// ffconcat playlist and re-muxes with `-c copy` (prior-art §1).
//
// This is the parent, not a program child. It never re-encodes — all the encoding happens in the
// per-program children the concat demuxer requests — which is what makes one channel cost one
// encode regardless of how many programs it plays.
func playoutSpawner(
	ffmpegBin string, publicURL func() string, token func() string, log *slog.Logger,
) playout.Spawner {
	return func(ctx context.Context, channelID string) (*playout.Process, error) {
		base := publicURL()
		if base == "" {
			// Without an absolute base the parent cannot fetch its own playlist: ffmpeg is a
			// separate process with no notion of "the origin this came from". Failing here with
			// a clear message beats emitting a URL that fails inside ffmpeg.
			return nil, fmt.Errorf("playout: server.public_url is not set, so ffmpeg cannot reach the playlist")
		}
		playlistURL := fmt.Sprintf("%s/playout/playlist/%s?token=%s",
			strings.TrimRight(base, "/"), url.PathEscape(channelID), url.QueryEscape(token()))
		return playout.Start(ctx, ffmpegBin, playout.ConcatArgs(playlistURL), log, nil)
	}
}
