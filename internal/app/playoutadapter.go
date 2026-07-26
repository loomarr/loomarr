package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/provision"
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

// titleReader is the one store method provenance needs.
//
// Narrowed to a single method deliberately, the same way cyclePreviewer is: the guide READS
// acquisition state and must not be able to mutate it. A structural guarantee beats a rule
// someone has to remember.
type titleReader interface {
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// playoutResolver answers "what is airing now, and where does ffmpeg read it from".
// clipPlayRecorder is the one-method slice of the store the resolver needs to count an
// airing. Narrow deliberately — the resolver has no other business writing.
type clipPlayRecorder interface {
	RecordClipPlay(ctx context.Context, clipPath string, at time.Time) error
}

type playoutResolver struct {
	engine cyclePreviewer
	lib    *library.Client
	now    func() time.Time
	// titles resolves a block's acquisition record for the grid's provenance line. Nil ⇒ the
	// line is simply absent, which is the right degradation: a guide without provenance is
	// still a guide.
	titles titleReader
	// clipPlays counts a filler clip having aired (V28). Nil ⇒ no counting, which is the
	// correct degradation: usage is telemetry and a channel must still play without it.
	clipPlays clipPlayRecorder

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

	// pods assembles the channel's commercial break (§10). The SAME PodPreviewer the API and
	// the reconciler use, so the ad that plays is the one the channel page previewed — §10's
	// one-assembler rule. Nil ⇒ breaks fall back to the offline card.
	pods api.PodPreviewer
	// fillerDir resolves a clip's relative id to a file on disk. A func so a settings change
	// applies without a restart, like the other live reads above.
	fillerDir func() string

	// ffmpegPath is the binary the capability probe executes.
	ffmpegPath func() string
	log        *slog.Logger
	// detectOnce / detected cache the measured encoder choice (detectedEncoder).
	//
	// Cached because Detect trial-encodes every candidate at ~5s apiece — fine once, far too
	// slow on the per-program path. NOT a plain field set at construction: probing eagerly
	// would add ~20s to every boot for a value most installs never override.
	detectOnce sync.Once
	detected   playout.Encoder
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

	// A BREAK GAP resolves to a real commercial (§10). Tunarr used to do this: the scheduler
	// leaves flex, and Tunarr played clips from a filler-list into it. Internal playout has no
	// such negotiator, so it must pick the clip itself — otherwise every break is dead air.
	if airing.Kind == schedule.SlotFiller {
		return r.airingFiller(ctx, channelID, airing, now)
	}

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

// BroadcastsBetween resolves a channel's programme timeline for the XMLTV guide (§9.1, V6b).
//
// Deliberately on the SAME type as AiringNow, reading the SAME CyclePreview: the guide and the
// encoder must agree, and the cheapest way to guarantee that is to give them one source rather
// than two that happen to match today.
//
// `at` is the window's START, not `now`. CyclePreview evaluates curation rules at an instant —
// a rule that switches the channel to horror at 21:00 changes the lineup — and a guide built at
// `now` would advertise the current rule's lineup for the whole window. Using `from` means the
// listings reflect what the rules said when the window opened; a window spanning a rule
// boundary is a known limitation rather than a silent wrong answer (the mid-window portion
// shows the earlier rule's programmes).
func (r *playoutResolver) BroadcastsBetween(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, from)
	if err != nil {
		return nil, err
	}
	bs := playout.BroadcastsBetween(slots, playoutEpoch(channelID), from, to)
	r.attachMetadata(ctx, bs)
	return bs, nil
}

// BroadcastsWithPending is BroadcastsBetween plus pending acquisitions, for the time grid
// (V13b). Same CyclePreview, same epoch, same arithmetic — only the projection differs, so the
// grid cannot disagree with the encoder about what airs when.
func (r *playoutResolver) BroadcastsWithPending(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, from)
	if err != nil {
		return nil, err
	}
	bs := playout.BroadcastsWithPending(slots, playoutEpoch(channelID), from, to)
	r.attachMetadata(ctx, bs)
	r.attachProvenance(ctx, bs)
	return bs, nil
}

// attachProvenance fills in each block's one-line "why is this here" (§12 hover card).
//
// Only for the GRID, never the XMLTV guide: an EPG lists what is on, and "acquiring · 62%" is
// an operator's answer, not a viewer's listing. That split is why this is a separate pass
// rather than part of attachMetadata.
//
// BEST-EFFORT, like the metadata pass: a store hiccup leaves the blocks exactly as they were.
// A hover card missing one line is far better than a guide that fails to load.
func (r *playoutResolver) attachProvenance(ctx context.Context, bs []playout.Broadcast) {
	if r.titles == nil || len(bs) == 0 {
		return
	}
	now := r.now()
	// One lookup per DISTINCT key: a channel airing six episodes of one series shares a
	// single acquisition record, so keying the cache on the provisioning key collapses those
	// to one read.
	cache := map[provision.Key]string{}
	for i := range bs {
		k := bs[i].Key
		if k == "" {
			continue // filler/flex have no acquisition to describe
		}
		if p, ok := cache[k]; ok {
			bs[i].Provenance = p
			continue
		}
		rec, err := r.titles.GetTitle(ctx, k)
		if err != nil {
			cache[k] = "" // remember the miss too, so one bad key is not re-read per block
			continue
		}
		p := provenanceOf(rec, now)
		cache[k] = p
		bs[i].Provenance = p
	}
}

// attachMetadata fills in descriptions, genres, years and ratings from the media server.
//
// ONE BULK CALL for the whole channel, which is the only reason this is affordable on a request
// a media server polls: Emby's `/Items?Ids=` takes a comma-separated list, and 120 episodes came
// back in 24ms on the dev stack. Per-item lookups would have been a round trip per programme and
// would have forced a cache.
//
// BEST-EFFORT BY DESIGN. A failure leaves the broadcasts exactly as they were — titles and times
// intact — because a guide with thin entries is far better than no guide. The media server is an
// external dependency on a path that must keep working when it is slow or briefly down.
func (r *playoutResolver) attachMetadata(ctx context.Context, bs []playout.Broadcast) {
	if r.lib == nil || len(bs) == 0 {
		return
	}
	ids := make([]string, 0, len(bs))
	for _, b := range bs {
		if b.LibraryItemID != "" {
			ids = append(ids, b.LibraryItemID)
		}
	}
	if len(ids) == 0 {
		return
	}

	meta, err := r.lib.ItemMetadataByID(ctx, ids)
	if err != nil && r.log != nil {
		// Logged, not returned: the partial map is still applied below. ItemMetadataByID
		// returns what it gathered alongside the error precisely so a slow page of a large
		// guide does not cost the whole document its descriptions.
		r.log.Debug("playout: guide metadata partially unavailable",
			"err", err, "resolved", len(meta), "wanted", len(ids))
	}
	for i := range bs {
		m, ok := meta[bs[i].LibraryItemID]
		if !ok {
			continue // removed from the library since the lineup was built; keep the title
		}
		bs[i].Description = m.Overview
		bs[i].Genres = m.Genres
		bs[i].Year = m.Year
		bs[i].Rating = m.OfficialRating
		bs[i].RuntimeMs = m.RuntimeMs
	}
}

// airingFiller resolves a break gap to ONE specific commercial file.
//
// The gap is a single slot on the timeline, but a pod is a SEQUENCE (bumper → ads → bumper),
// so this walks the pod by the offset already computed into the break and returns whichever
// clip covers that instant. Same shape as AiringAt one level down — and it must be, for the
// same reason: two viewers asking mid-break have to get the same clip at the same position.
//
// The pod comes from PodPreviewer, which is the SAME assembler and the SAME seed the reconciler
// and the UI preview use (§10's one-assembler rule). So the commercial that plays is the one the
// channel page promised, not a second opinion.
func (r *playoutResolver) airingFiller(
	ctx context.Context, channelID string, gap playout.Airing, now time.Time,
) (playout.Airing, string, error) {
	if r.pods == nil || r.fillerDir == nil {
		// Filler unconfigured: the break becomes the offline card rather than an error. A
		// channel with no commercials should still play.
		return playout.Airing{Kind: schedule.SlotFlex}, "", nil
	}

	// PreviewAt, not Preview: the pod is seeded from THIS break's start, so consecutive
	// breaks play different adverts — and the guide's hover card, which calls the same
	// method with the same start, lists exactly what will air here.
	//
	// gap.Offset is how far into the break we are, so the break began that long ago.
	breakStart := now.Add(-gap.Offset).UnixMilli()
	pod, err := r.pods.PreviewAt(ctx, channelID, breakStart)
	if err != nil || len(pod.Entries) == 0 {
		// No pool, or the assembler could not fill this break. Not an error: §10's ladder
		// bottoms out at "nothing matched", and a channel must not fail to tune because it has
		// no ads.
		return playout.Airing{Kind: schedule.SlotFlex}, "", nil
	}

	// Walk the pod to the instant we are at INSIDE the break. gap.Offset is how far into the
	// break the wall clock has reached, which AiringAt already computed.
	into := gap.Offset
	for _, e := range pod.Entries {
		d := time.Duration(e.DurationMs) * time.Millisecond
		if d <= 0 {
			continue // a clip with no duration cannot occupy time; skip rather than divide by it
		}
		if into < d {
			// The embedded fallback bumper card has no file — it is generated, not played.
			if e.Path == "" {
				return playout.Airing{Kind: schedule.SlotFlex}, "", nil
			}
			// ⚠ ClipPath is the containment check, not a join: the id comes from the database
			// and a crafted `../` would otherwise stream an arbitrary file off the host.
			full, perr := filler.ClipPath(r.fillerDir(), e.Path)
			if perr != nil {
				return playout.Airing{Kind: schedule.SlotFlex}, "", nil
			}
			// Count the airing (V28). THIS is the honest write point, and the two
			// tempting alternatives are both wrong:
			//   - pod ASSEMBLY re-runs on every 10m reconcile sweep, so it counts what was
			//     scheduled, over and over, not what aired;
			//   - the /playout/program HANDLER would count per tune-in, so three viewers on
			//     one break would be three plays.
			// Here the resolver is answering one demuxer request for one item, and Attach
			// starts at most one parent encoder per channel — so this fires once per clip
			// actually encoded. `into == 0` restricts it to the item's START rather than every
			// mid-clip re-resolve.
			//
			// ⚠ Internal playout only. A Tunarr-backed channel airs its filler through Tunarr,
			// which never reports back, so those clips stay at zero — "not counted here", not
			// "never played". The DTO says which.
			if into == 0 && r.clipPlays != nil {
				if err := r.clipPlays.RecordClipPlay(ctx, e.Path, now); err != nil {
					// Telemetry, never correctness: a failed count must not stop a break from
					// airing. Logged at debug because a pruned clip is an ordinary race.
					_ = err
				}
			}
			return playout.Airing{
				Kind: schedule.SlotProgram, // playable: the handler encodes it like any program
				// Source, not LibraryItemID: this is a local file, not a media-server item.
				// Playable() checks Source for exactly this case.
				Source:    full,
				Title:     e.Name,
				Offset:    into,
				Remaining: d - into,
			}, full, nil
		}
		into -= d
	}

	// The pod is shorter than the break gap. Real: a 30s break with 20s of clips. The remainder
	// is the offline card rather than a repeat, because repeating would mean the same ad twice
	// in one break — worse than a moment of card.
	return playout.Airing{Kind: schedule.SlotFlex}, "", nil
}

// Profile is the encode profile for the next program, resolved against live load.
//
// Called once per program (each child is a fresh process), so the ladder adapts as channels come
// and go: the first channel on an idle box gets the top rung, and a fifth channel starting up
// steps everyone down as their next program begins. That is the "best picture the hardware
// sustains, then adapt" policy §9.1 states, and it is only implementable because the child
// processes are short-lived.
func (r *playoutResolver) Profile(ctx context.Context) playout.Profile {
	enc := playout.Encoder(r.encoder())
	if enc == "" {
		// No operator override ⇒ ASK THE HARDWARE, once.
		//
		// This used to fall straight through to libx264 with a comment claiming the
		// capability prober's choice "was stored at wizard time" — but nothing ever stored
		// it, so the fallback was unconditional and a box with a working GPU silently
		// encoded in software forever. Detect trial-encodes each candidate, so its answer is
		// measured rather than inferred from `ffmpeg -encoders` (which lists encoders the
		// hardware cannot actually run — the exact trap that took a live channel down: the
		// host listed h264_vulkan, the container had no /dev/dri).
		enc = r.detectedEncoder(ctx)
	}
	return playout.Resolve(playout.TierFor(r.tier()), enc, r.capacity(), r.activeChannels())
}

// detectedEncoder returns the best encoder that actually WORKS here, probing once.
//
// Lazily and exactly once, which is the only workable timing: Detect trial-encodes every
// candidate (~5s each), so it is far too slow for the per-program path and would add ~20s to
// every boot if done eagerly — for a value most installs never need. The first program to need
// it pays; everything after reads the cached answer.
//
// An operator who changes their hardware (adds GPU passthrough, which is a compose change and
// needs a restart anyway) gets a fresh probe on the next start.
func (r *playoutResolver) detectedEncoder(ctx context.Context) playout.Encoder {
	r.detectOnce.Do(func() {
		bin := r.ffmpegPath()
		if bin == "" {
			bin = "ffmpeg"
		}
		cap := playout.Detect(ctx, bin, playout.DefaultProfile())
		r.detected = cap.Chosen
		if r.log != nil {
			// INFO, not DEBUG: which encoder a box settled on is the first thing anyone asks
			// when playout is slow, and the per-candidate reasons explain WHY a GPU was
			// skipped ("Device creation failed" vs "not in this ffmpeg build").
			skipped := make([]string, 0, len(cap.All))
			for _, c := range cap.All {
				if !c.Works {
					skipped = append(skipped, string(c.Encoder)+": "+c.Err)
				}
			}
			r.log.Info("playout: encoder probed",
				"chosen", cap.Chosen, "measured_max_channels", cap.MaxChannels,
				"skipped", skipped)
		}
	})
	return r.detected
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
