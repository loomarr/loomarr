// Package playoutresolve is the adapter that makes internal playout work (§9.1): where three
// separately-verified pieces finally compose.
//
//	store.Channel.Desired  → the accepted cycle reconciliation committed for broadcast
//	playout.AiringAt       → which program that puts on right now, and at what offset
//	LibraryPlayingFile     → the library item's file, which ffmpeg reads directly
//
// It lives under internal/app rather than internal/playout on purpose. playout is the mechanism
// (encoders, args, sessions) and must not know about stores, media servers, or settings; this
// package is the wiring, and wiring belongs to the composition root. Only internal/app builds it.
package playoutresolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
)

// CyclePreviewer is the scheduling surface the resolver needs — satisfied by *channels.Engine.
//
// Narrowed to reads deliberately: the resolver must not be able to reconcile, push to Tunarr, or
// mutate anything. Playout is a READ of the schedule, and a narrow interface makes that
// structural rather than a rule someone has to remember.
type CyclePreviewer interface {
	CyclePreview(ctx context.Context, channelID string, at time.Time) (
		resolvedAt time.Time, slots []schedule.Slot, active schedule.ActiveRuleAttribution,
		window time.Duration, err error)
	// RollingWindow and CarriesOver are the inputs of the rolling-window turn (#1675), resolved
	// exactly as reconcile resolves them, so the resolver turns the window where reconcile will.
	RollingWindow(policy schedule.ChannelPolicy, at time.Time) (time.Duration, *time.Location)
	CarriesOver(ctx context.Context, policy schedule.ChannelPolicy) (bool, error)
	// Calendar is the holiday calendar the arrangement reads, so the cycle cache keys on it.
	Calendar() schedule.Calendar
}

// TitleReader is the one store method provenance needs.
//
// Narrowed to a single method deliberately, the same way cyclePreviewer is: the guide READS
// acquisition state and must not be able to mutate it. A structural guarantee beats a rule
// someone has to remember.
type TitleReader interface {
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// ClipPlayRecorder is the one-method slice of the store the resolver needs to count an
// airing. `at` is the scheduled clip start, which makes repeated resolves of one airing
// idempotent in the store; `recordedAt` is the wall clock. Narrow deliberately — the resolver
// has no other business writing.
type ClipPlayRecorder interface {
	RecordClipPlay(ctx context.Context, channelID, clipHash string, at, recordedAt time.Time) (bool, error)
}

// AiringRecorder stamps that a PROGRAMME aired (§5, programming-design §3.1) — the recency
// signal placement ranks on. Narrowed to the one write, like every other store slice here.
type AiringRecorder interface {
	RecordAiring(ctx context.Context, channelID string, key provision.Key, libraryItemID string, airedAt, recordedAt time.Time) error
}

// ChannelReader is the one store method both broadcast and guide need. Broadcast reads Desired,
// the accepted cycle reconciliation committed; guide also fingerprints Lineup and Policy when it
// forecasts a future rolling window (see cyclecache.go). Narrowed like the readers above — both
// are reads, and neither path may mutate the channel it observes.
type ChannelReader interface {
	GetChannel(ctx context.Context, id string) (channelstore.Channel, error)
}

// CodecWriter persists a channel's computed broadcast codec (§9.1 V50). Narrowed to the one
// targeted column write — the resolver samples + probes the lineup, but has no other business
// mutating a channel row (the binder is the one lineup writer).
type CodecWriter interface {
	SetChannelBroadcastCodec(ctx context.Context, id string, expectedRevision int64, codec string) (int64, error)
}

// Resolver answers "what is airing now, and where does ffmpeg read it from". Build one with New;
// the zero value is not wired to anything.
type Resolver struct {
	engine  CyclePreviewer
	now     func() time.Time
	metrics *metrics.Recorder
	// titles resolves a block's acquisition record for the grid's provenance line. Nil ⇒ the
	// line is simply absent, which is the right degradation: a guide without provenance is
	// still a guide.
	titles TitleReader
	// clipPlays counts a filler clip having aired (V28). Nil ⇒ no counting, which is the
	// correct degradation: usage is telemetry and a channel must still play without it.
	clipPlays ClipPlayRecorder
	// airings records that a PROGRAMME aired, feeding recency-aware placement
	// (programming-design §3.1). Nil ⇒ no history, and placement degrades to the positional
	// rotation.
	airings AiringRecorder

	// tier / encoder / capacity are read live so an operator's Settings change applies to the
	// NEXT program rather than requiring a restart. Each programme item is its own encoder, so
	// "the next program" is at most one program away — which makes hot-apply cheap.
	tier    func() string
	encoder func() string
	// toneCurve is playout.tone_curve: the budget prices HDR by it and the probe measures it.
	toneCurve func() string

	// pods assembles the channel's commercial break (§10). The SAME assembler the API and
	// the reconciler use, so the ad that plays is the one the channel page previewed — §10's
	// one-assembler rule. Nil ⇒ breaks fall back to the offline card.
	pods PodPreviewer
	// fillerDir resolves a clip's relative id to a file on disk. It is immutable for the
	// generation: catalog paths, scan and playout must never interpret one row under different
	// roots while a saved layout change is waiting on restart.
	fillerDir string

	// probeFormat probes a source's codec/format for the direct-play copy decision (probe.go).
	// Nil ⇒ treat as transcode-required (safe: correctness over speed when we cannot probe).
	probeFormat playout.FormatProber

	// ffmpegPath is the binary the capability probe executes.
	ffmpegPath func() string
	// capabilityRoot is the persistent prepared root that carries bounded, regenerable host
	// capability evidence across restarts. Nil/empty keeps ordinary full detection.
	capabilityRoot func() string
	// gpuName reports the primary GPU's name so the capability probe can prefer that vendor's
	// native encoder (Detect's gpuVendor arg). Nil or "" ⇒ unknown GPU ⇒ the cross-vendor default
	// order, so an install without the wiring still detects an encoder, just without the native hint.
	gpuName func() string
	// audioLanguage is the operator's preferred audio track language (§9.1, `eng` by default),
	// read live like every other setting. Empty ⇒ the file's first track, which is how a
	// channel once played a film in Russian.
	audioLanguage func() string
	// measurer records first-play stream facts and queues background analysis (keyframes, loudness,
	// break candidates) per source revision. Nil ⇒ playout probes each time and stores nothing.
	measurer sourceMeasurer
	// analyses reads what the measurer stored: the keyframe index a still seeks with, the scene
	// fades mid-roll placement uses (§10), and the active picture the channel watermark anchors to.
	// Nil ⇒ ffmpeg's container seek, no mid-roll breaks (every programme airs whole), and the bug
	// anchored to the fitted frame.
	analyses inventory.AnalysisReader
	// probeSource returns the shared ffprobe superset so the audio choice and durable technical
	// observation come from one process. Nil ⇒ track 0, preserving best-effort playout.
	probeSource playout.SourceProber
	// inventory is Loomarr's durable provider-neutral source observation (§5 V66). Audio selection
	// reads it before Library or ffprobe I/O; nil preserves the safe direct-probe fallback.
	inventory inventory.Service
	// libraryAudio is the Library's tracks for a file the scanner read (#2151 decision 47). Audio
	// selection reads it first, while the file is as the scanner read it; nil skips it.
	libraryAudio LibraryAudioReader
	// libraryFile is the file a Library item plays (#2151 step 15): a programme keyed by a library
	// item id reads it directly, with no media server. nil, or any other id, resolves through the
	// media server.
	libraryFile LibraryFileReader
	// libraryVideo is a Library item's video format as the scanner read it, for lineup formats and
	// still tone mapping; nil, or any other id, reads the inventory.
	libraryVideo LibraryVideoReader
	// itemFacts answers the guide's metadata for a library item id from the Library's tables; nil
	// leaves the guide without descriptions, and any other id has none.
	itemFacts store.LibraryItemFactsReader
	// probeTracks lists a source's audio AND subtitle tracks for the Watch pickers (§9.1, V46) —
	// the options come from the airing media, not a list. Nil ⇒ empty tracks, so the pickers show
	// only the current channel default rather than blocking.
	probeTracks        playout.TrackProber
	processDiagnostics *diagnostics.ProcessManager
	log                *slog.Logger
	// channels reads the channel the arranged-cycle cache fingerprints, and cycles is that
	// cache (cyclecache.go). Both nil ⇒ the guide computes every cycle live, so tests and any
	// install that skips the wiring still get correct (merely slower) listings.
	channels ChannelReader
	// codecs persists the computed broadcast codec after a bind samples the lineup (§9.1 V50).
	// Nil ⇒ ComputeChannelCodec still returns the measured codec but skips the write — so a
	// caller that only wants the value (or an install without the wiring) degrades to "compute,
	// don't store", never to a failed bind.
	codecs CodecWriter
	cycles *cycleCache
	// detectOnce / detected cache the measured encoder choice (detectedEncoder). The first live
	// demand synchronously checks only the persisted evidence fingerprint, then detectStart validates
	// a match (or performs the full benchmark on a miss) without making playback wait.
	//
	// Process-cached because Detect trial-encodes every candidate at ~5s apiece — fine once, far
	// too slow on the per-program path. A verified hardware result can also survive restart through
	// capabilityRoot when its cheap host fingerprint still matches; a bounded real validation then
	// runs asynchronously.
	detectOnce     sync.Once
	detectStart    sync.Once
	detectEvidence sync.Once
	inputsOnce     sync.Once
	detectReady    atomic.Bool
	detectedMu     sync.RWMutex
	detectContext  context.Context
	detected       encode.Encoder
	maxChannels    atomic.Int64
	// encodeHostBytes is the capability probe's measured per-encode host memory (0 = unmeasured).
	encodeHostBytes atomic.Int64
	capabilityBin   string
	capabilityGPU   string
	capabilityPath  string
	// Test seam for the cheap persisted-evidence identity check. Nil fingerprints the real
	// FFmpeg/GPU/profile and reads the evidence; it starts no encoder trial or full benchmark.
	loadCapabilityEvidence func(context.Context) (playout.Capacity, bool)
	// measuredCosts is the class probe's published cost table (probeCapacity); onTonemap receives its
	// tone-map self-check.
	measuredCosts atomic.Pointer[playout.MeasuredCosts]
	onTonemap     func(playout.TonemapCheck)
}

// PodPreviewer is the slice of the filler pod assembler the resolver airs a break from. It is
// satisfied by the assembler the API previews with, so the pod that plays is the pod that was
// previewed (§10).
type PodPreviewer interface {
	// PreviewAt assembles the pod for the break starting at slot.Start, to the slot's own length.
	PreviewAt(ctx context.Context, channelID string, slot pod.BreakSlot) (pod.Pod, error)
}

// Config is what New wires a Resolver from. Every field is optional: an unset reader or setting
// degrades the way its Resolver field documents (no provenance line, no clip counting, the
// cross-vendor encoder order, and so on).
type Config struct {
	// Engine is the schedule the resolver reads.
	Engine CyclePreviewer
	// Now is the clock.
	Now func() time.Time
	// Metrics receives the filler rotation counters.
	Metrics *metrics.Recorder
	// DetectContext bounds the background encoder detection; it is the application's lifetime.
	DetectContext context.Context
	// Titles resolves a block's acquisition record for the grid's provenance line.
	Titles TitleReader
	// ClipPlays counts a filler clip having aired (V28).
	ClipPlays ClipPlayRecorder
	// Airings records that a programme aired, feeding recency-aware placement.
	Airings AiringRecorder
	// Channels reads the channel the arranged-cycle cache fingerprints.
	Channels ChannelReader
	// Codecs persists the computed broadcast codec (§9.1 V50).
	Codecs CodecWriter
	// LibraryAudio is the Library's tracks for a file the scanner read.
	LibraryAudio LibraryAudioReader
	// LibraryFile is the file a Library item plays.
	LibraryFile LibraryFileReader
	// LibraryVideo is a Library item's video format as the scanner read it.
	LibraryVideo LibraryVideoReader
	// ItemFacts answers the guide's metadata for a library item id.
	ItemFacts store.LibraryItemFactsReader

	// Tier, Encoder and ToneCurve read playout.quality_tier, playout.encoder and
	// playout.tone_curve live.
	Tier, Encoder, ToneCurve func() string
	// FillerDir is the clip root, immutable for the generation.
	FillerDir string
	// FFmpegPath is the binary the capability probe executes.
	FFmpegPath func() string
	// CapabilityRoot is the persistent root that carries host capability evidence across restarts.
	CapabilityRoot func() string
	// GPUName reports the primary GPU's name for the encoder chooser's vendor-native hint.
	GPUName func() string
	// AudioLanguage is the operator's preferred audio track language, read live.
	AudioLanguage func() string

	// Inventory is Loomarr's durable provider-neutral source observation.
	Inventory inventory.Service
	// ProbeSource, ProbeTracks and ProbeFormat are the ffprobe-backed source, track and format probes.
	ProbeSource playout.SourceProber
	ProbeTracks playout.TrackProber
	ProbeFormat playout.FormatProber
	// ProcessDiagnostics tracks the processes the resolver starts.
	ProcessDiagnostics *diagnostics.ProcessManager
	// Log is the structured logger; nil logs nothing.
	Log *slog.Logger
}

// New returns a Resolver wired from cfg, with its arranged-cycle cache. The pod assembler, the
// source measurer and the tone-map observer are attached afterwards (SetPods, SetMeasurer,
// SetTonemapObserver) because the objects they come from are built later.
func New(cfg Config) *Resolver {
	return &Resolver{
		engine: cfg.Engine, now: cfg.Now,
		metrics:            cfg.Metrics,
		detectContext:      cfg.DetectContext,
		titles:             cfg.Titles,
		clipPlays:          cfg.ClipPlays,
		airings:            cfg.Airings,
		channels:           cfg.Channels,
		codecs:             cfg.Codecs,
		libraryAudio:       cfg.LibraryAudio,
		libraryFile:        cfg.LibraryFile,
		libraryVideo:       cfg.LibraryVideo,
		itemFacts:          cfg.ItemFacts,
		cycles:             newCycleCache(time.Now),
		tier:               cfg.Tier,
		encoder:            cfg.Encoder,
		toneCurve:          cfg.ToneCurve,
		fillerDir:          cfg.FillerDir,
		ffmpegPath:         cfg.FFmpegPath,
		capabilityRoot:     cfg.CapabilityRoot,
		gpuName:            cfg.GPUName,
		audioLanguage:      cfg.AudioLanguage,
		inventory:          cfg.Inventory,
		probeSource:        cfg.ProbeSource,
		probeTracks:        cfg.ProbeTracks,
		probeFormat:        cfg.ProbeFormat,
		processDiagnostics: cfg.ProcessDiagnostics,
		log:                cfg.Log,
	}
}

// SetPods attaches the assembler a break airs its commercials from. Until it is set, breaks fall
// back to the offline card.
func (r *Resolver) SetPods(pods PodPreviewer) { r.pods = pods }

// SetMeasurer attaches the source measurer (NewMeasurer) and the analysis store it writes, which
// the resolver reads back for keyframes, scene fades and the active picture.
func (r *Resolver) SetMeasurer(measurer *measure.Measurer, analyses inventory.AnalysisReader) {
	r.measurer = measurer
	r.analyses = analyses
}

// SetTonemapObserver receives the capacity probe's tone-map self-check.
func (r *Resolver) SetTonemapObserver(observe func(playout.TonemapCheck)) { r.onTonemap = observe }

// FillerDir is the clip root this resolver reads, fixed for the generation.
func (r *Resolver) FillerDir() string { return r.fillerDir }

// MaxChannels is the concurrent-encode capacity the encoder detection measured, 0 until it has.
func (r *Resolver) MaxChannels() int { return int(r.maxChannels.Load()) }

// DetectReady reports whether encoder detection has published its result.
func (r *Resolver) DetectReady() bool { return r.detectReady.Load() }

// SetMaxChannels publishes a measured capacity without running the encoder trial that would. It
// is the seam for tests that need a capacity without ffmpeg.
func (r *Resolver) SetMaxChannels(n int) { r.maxChannels.Store(int64(n)) }

// SetMeasuredCosts publishes a class-cost table without running the probe that would. It is the
// seam for tests that need measured costs without ffmpeg.
func (r *Resolver) SetMeasuredCosts(costs *playout.MeasuredCosts) { r.measuredCosts.Store(costs) }

// MissingLadderInputs names the settings readers the output ladder calls unguarded that this
// Resolver was built without; a viewer tuning in would panic on any of them.
func (r *Resolver) MissingLadderInputs() []string {
	var missing []string
	if r.tier == nil {
		missing = append(missing, "tier")
	}
	if r.encoder == nil {
		missing = append(missing, "encoder")
	}
	return missing
}

// AiringNow resolves the channel's current program and its ffmpeg input URL.
//
// It reads the persisted Desired cycle that reconciliation ACCEPTED. CyclePreview is an authoring
// and forecasting surface: it includes mutable availability and airing-history inputs, so invoking
// it here would let the deck change at a programme EOF. Desired is also what makes the answer
// survive a process restart; the deterministic channel epoch supplies the matching wall-clock
// position.
func (r *Resolver) AiringNow(ctx context.Context, channelID string) (playout.Airing, string, error) {
	return r.AiringAt(ctx, channelID, r.now())
}

// AiringAt is AiringNow at a given instant. The channel packager asks for the item that airs at
// its timeline's end, up to its run-ahead (~12 s) after now.
func (r *Resolver) AiringAt(ctx context.Context, channelID string, now time.Time) (playout.Airing, string, error) {
	slots, epoch, err := r.acceptedCycle(ctx, channelID, now)
	if err != nil {
		return playout.Airing{}, "", err
	}

	airing := playout.AiringAt(slots, epoch, now)
	airing.ScheduleBlockID = playout.ScheduledBlockID(channelID, airing.StartedAt, airing.Kind, airing.Identity)

	// A BREAK GAP resolves to a real commercial (§10). The scheduler leaves flex, and a Tunarr
	// channel has Tunarr play clips from a filler-list into it. Internal playout has no such
	// negotiator, so it must pick the clip itself — otherwise every break is dead air.
	if airing.Kind == schedule.SlotFiller {
		return r.airingFiller(ctx, channelID, airing, now)
	}

	if !airing.Playable() {
		// Not an error: an empty lineup, or one where nothing has landed yet, is a real state.
		// The handler renders it as the offline card.
		return airing, "", nil
	}

	// Resolve the input ffmpeg reads: the library item's file, directly. This is where direct play
	// begins — reading the file lets the encoder `-c copy` it.
	input := r.itemInput(ctx, airing.LibraryItemID)
	if input == "" {
		// The lineup names an item whose file is gone or unreadable, or an id the Library does
		// not hold. Reporting it as "nothing airing" rather than an error means the channel shows
		// the offline card instead of failing to tune — the same outcome the viewer would get
		// from an error, minus the retry storm.
		return playout.Airing{Kind: schedule.SlotFlex}, "", nil
	}
	return airing, input, nil
}

// servedWriteTimeout bounds the airing-history write RecordAiringServed makes after its caller's
// context may have ended.
const servedWriteTimeout = 5 * time.Second

// RecordAiringServed records that a viewer received segments of an airing AiringAt resolved at
// `resolvedAt`. It is the ONLY writer of airing history and of a filler clip's play count: every
// resolve is a read (the admin Info page's /tracks, the packager's lookups for a neighbour warm),
// and recording on a read rested ads on channels nobody watched.
//
// A programme is recorded under its provisioning key (§5, programming-design §3.1), the input
// recency-aware placement ranks on. It is stamped with its START (`resolvedAt - Offset`), not with
// the serve time, so the value means "when did this programme begin airing" however late anyone
// tuned in. The row is an upsert holding the last airing, so the packager re-resolving after an
// encoder restart is idempotent. It is also stamped with when it was observed, never earlier than
// the start, because placement reads history as of a window's start (#1674): the packager resolves
// up to its run-ahead early, and a programme that opens a window must not count toward that
// window's arrangement.
//
// A filler clip is counted by HASH (`RecordClipPlay` is `WHERE hash = ?`; keyed by path every
// counter stayed at zero) at its SCHEDULED start, which the store makes idempotent; an entry with
// no hash, the generated bumper card, is not counted. Internal playout only: a Tunarr-backed
// channel airs its filler through Tunarr, which never reports back, so those clips read "not
// counted here", not "never played".
//
// Telemetry, never correctness: a failed write is logged and the airing carries on.
func (r *Resolver) RecordAiringServed(ctx context.Context, channelID string, airing playout.Airing, resolvedAt time.Time) {
	// The serve that fired this runs under the channel packager's context, which ends the moment the
	// last viewer leaves; the write has already been earned, so it runs detached and bounded.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), servedWriteTimeout)
	defer cancel()
	switch {
	case airing.Kind == schedule.SlotFiller:
		if airing.Identity == "" || r.clipPlays == nil {
			return
		}
		recorded, err := r.clipPlays.RecordClipPlay(ctx, channelID, airing.Identity, airing.StartedAt, r.now())
		if err != nil {
			if r.log != nil {
				// Debug: a pruned clip is an ordinary race.
				r.log.Debug("playout: clip play not recorded", "channel", channelID, "clip", airing.Identity, "err", err)
			}
			return
		}
		if recorded && r.metrics != nil {
			r.metrics.FillerRotationAired(airing.RecentRepeat, airing.RecentRepeat && !airing.RotationPinned, airing.RotationPinned)
		}
	case r.airings != nil && airing.Key != "":
		startedAt := resolvedAt.Add(-airing.Offset)
		recordedAt := r.now()
		if startedAt.After(recordedAt) {
			recordedAt = startedAt
		}
		if err := r.airings.RecordAiring(ctx, channelID, airing.Key, airing.LibraryItemID, startedAt, recordedAt); err != nil && r.log != nil {
			r.log.Warn("playout: airing not recorded", "channel", channelID, "key", airing.Key, "err", err)
		}
	}
}

// StillAiring is the cold-channel still's view of the schedule (playout.StillAiringResolver): the
// programme on now, with its source resolved lazily. Unlike AiringNow it writes nothing (no airing
// history, no filler pick), because showing a picture is not airing it. It runs on every still
// request, so it reads only the accepted cycle; the source lookup, which can cost a media-server
// round trip, runs once per airing on the still cache's miss. A break has no still: resolving its
// clip would pick, and count, filler.
func (r *Resolver) StillAiring(ctx context.Context, channelID string) (playout.StillAiring, bool, error) {
	now := r.now()
	slots, epoch, err := r.acceptedCycle(ctx, channelID, now)
	if err != nil {
		return playout.StillAiring{}, false, err
	}
	airing := playout.AiringAt(slots, epoch, now)
	if airing.Kind == schedule.SlotFiller || !airing.Playable() {
		return playout.StillAiring{}, false, nil
	}
	itemID := airing.LibraryItemID
	return playout.StillAiring{
		Key:    playout.ScheduledBlockID(channelID, airing.StartedAt, airing.Kind, airing.Identity),
		Offset: airing.Offset,
		At:     airing.StartedAt,
		Source: func(ctx context.Context) (playout.StillSource, bool, error) {
			input := r.itemInput(ctx, itemID)
			if input == "" {
				return playout.StillSource{}, false, nil
			}
			source := playout.StillSource{Input: input, HDR: r.inventoryHDR(ctx, itemID)}
			source.Keyframe, source.Indexed = r.indexedKeyframe(ctx, input, airing.Offset)
			return source, true, nil
		},
	}, true, nil
}

// indexedKeyframe is the direct file's last keyframe at or before offset, from the keyframe index
// Loomarr measured for the file's current bytes. ok=false for a stream URL, an unmeasured file, or
// an index from older bytes; a still then relies on ffmpeg's container seek, and a packager
// tune-in on its accurate seek from the exact offset.
func (r *Resolver) indexedKeyframe(ctx context.Context, input string, offset time.Duration) (time.Duration, bool) {
	if r.analyses == nil {
		return 0, false
	}
	source, ok := r.knownLocalSource(ctx, input)
	if !ok {
		return 0, false
	}
	analysis, found, err := r.analyses.InventoryAnalysis(ctx, source.ID)
	if err != nil || !found || analysis.Revision != source.Revision {
		return 0, false
	}
	keyframe, ok := analysis.KeyframeAtOrBefore(offset.Milliseconds())
	if !ok {
		return 0, false
	}
	return time.Duration(keyframe.PTSMs) * time.Millisecond, true
}

// inventoryHDR reports whether the item's playing file carries an HDR transfer (PQ/HLG), as the
// scanner read its header (never ffprobe). Unknown is false: an untone-mapped still is dim, not
// broken.
func (r *Resolver) inventoryHDR(ctx context.Context, libraryItemID string) bool {
	format, ok := r.libraryFormat(ctx, libraryItemID)
	return ok && format.HDR()
}

// acceptedCycle is the broadcast commit boundary: the cycle the channel airs at `at` and the
// instant it is walked from. Reconciliation owns the write; the encoder and the guide own reads.
// Keeping the seam here makes it impossible for a programme boundary to accidentally call the
// mutable authoring preview again.
func (r *Resolver) acceptedCycle(ctx context.Context, channelID string, at time.Time) ([]schedule.Slot, time.Time, error) {
	if r.channels == nil {
		return nil, time.Time{}, errors.New("read accepted channel schedule: channel reader is not configured")
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read accepted channel schedule %s: %w", channelID, err)
	}
	c, err := r.airingCycle(ctx, ch, at)
	return c.slots, c.epoch, err
}

// airingCycle is one channel's accepted cycle as it airs at `at`, plus the rolling-window inputs
// the guide needs to walk past it.
type airingCycle struct {
	slots       []schedule.Slot
	epoch       time.Time
	window      time.Duration
	zone        *time.Location
	carriesOver bool
}

// airingCycle resolves what the channel airs at `at`: normally the persisted Desired cycle walked
// from the playout anchor.
//
// The one exception is the rolling-window turn (#1675). Once the programme crossing the window
// boundary has ended, the next window airs from that end whether or not reconcile has run yet to
// commit it. Until it does, the next window is arranged here with CyclePreview, the computation
// reconcile commits, so the handoff lands when the programme ends rather than at the next
// reconcile (up to its cooldown later), which would swap the slice in mid-programme. Reconcile
// applies the same rule (playout.WindowTurn) and commits the same anchor, and from then on this
// reads Desired again.
func (r *Resolver) airingCycle(ctx context.Context, ch channelstore.Channel, at time.Time) (airingCycle, error) {
	epoch, err := effectivePlayoutAnchor(ch)
	if err != nil {
		return airingCycle{}, err
	}
	c := airingCycle{slots: ch.Desired, epoch: epoch}
	if r.engine == nil { // no scheduler wired: nothing can turn the window, so Desired airs as is
		return c, nil
	}
	c.window, c.zone = r.engine.RollingWindow(ch.Policy, at)
	if c.carriesOver, err = r.engine.CarriesOver(ctx, ch.Policy); err != nil {
		return airingCycle{}, fmt.Errorf("read accepted channel schedule %s: %w", ch.ID, err)
	}
	if !c.carriesOver {
		return c, nil
	}
	_, turnEpoch, turned := playout.WindowTurn(ch.Desired, epoch, c.window, c.zone, at)
	if !turned {
		return c, nil
	}
	_, slots, _, _, err := r.engine.CyclePreview(ctx, ch.ID, at)
	if err != nil {
		return airingCycle{}, fmt.Errorf("arrange channel %s's next window: %w", ch.ID, err)
	}
	c.slots, c.epoch = slots, turnEpoch
	return c, nil
}

// ComputeChannelCodec derives the channel's uniform BROADCAST CODEC (§9.1 V50) from its library
// content: it samples the channel's program slots, probes each file's video codec, and takes the
// MAJORITY (Q2). The result is what the timeline normalizes to — the show that already matches
// copies, everything else (minority-codec titles, all filler) transcodes to match — which is what
// keeps the stream single-codec and therefore HEVC-fMP4-legal.
//
// Called at CURATION (after a bind writes the lineup), not on the play hot path: the codec is
// stored (Q1) so the session start reads a column, not a probe. It reuses the SAME resolve+probe
// path AiringNow uses (CyclePreview → itemInput → probeFormat), so the codec we store is
// measured from the very files we broadcast — it cannot drift from what actually plays.
//
// Returns BroadcastCodecH264 as the safe answer whenever it cannot measure (no probe wired, no
// resolvable files, an empty/all-pending lineup): h264/TS plays on every client, so an
// un-measurable channel stays maximally compatible rather than guessing HEVC. Ties break to h264
// for the same reason — a 50/50 channel is not clearly HEVC-dominant.
//
// If r.codecs is wired the measured codec is persisted; the measured value is returned either way.
func (r *Resolver) ComputeChannelCodec(ctx context.Context, channelID string) (string, error) {
	if r.codecs == nil {
		return r.measureChannelCodec(ctx, channelID), nil
	}
	if r.channels == nil {
		return channelstore.BroadcastCodecH264, errors.New("persist broadcast codec: channel reader is not configured")
	}

	// Measuring spans cycle construction, library resolution, and several probes. A lineup can
	// change during that window, so the derived codec is only valid when the channel revision is
	// unchanged on both sides of the measurement. The targeted write uses that same revision and
	// advances it, preventing either a stale probe or a full-row writer from overwriting the other.
	const maxCodecWriteAttempts = 4
	codec := channelstore.BroadcastCodecH264
	for attempt := 0; attempt < maxCodecWriteAttempts; attempt++ {
		before, err := r.channels.GetChannel(ctx, channelID)
		if err != nil {
			return codec, fmt.Errorf("read channel %s before codec measurement: %w", channelID, err)
		}
		codec = r.measureChannelCodec(ctx, channelID)
		after, err := r.channels.GetChannel(ctx, channelID)
		if err != nil {
			return codec, fmt.Errorf("read channel %s after codec measurement: %w", channelID, err)
		}
		if before.Revision != after.Revision {
			continue
		}
		if _, err := r.codecs.SetChannelBroadcastCodec(ctx, channelID, after.Revision, codec); err == nil {
			return codec, nil
		} else if !errors.Is(err, channelstore.ErrChannelStale) {
			return codec, fmt.Errorf("persist broadcast codec for %s: %w", channelID, err)
		}
		if err := ctx.Err(); err != nil {
			return codec, err
		}
	}
	return codec, fmt.Errorf("persist broadcast codec for %s: channel kept changing", channelID)
}

// ChannelCodec reads the channel's STORED broadcast codec (§9.1 V50) — the hot-path read the play
// URL and program routing use to decide the served container + the transcode target. It is a plain
// column read (set at curation by ComputeChannelCodec / backfilled at boot), NOT a probe: the whole
// point of storing it (Q1) is that a session start costs a row read, not N ffprobes.
//
// Defaults to BroadcastCodecH264 on any miss (no channel reader wired, channel not found, or an
// empty column on an un-backfilled row) — h264/TS is the maximally-compatible fallback, so a
// lookup failure degrades a channel to "plays everywhere", never to a black screen.
func (r *Resolver) ChannelCodec(ctx context.Context, channelID string) string {
	if r.channels == nil {
		return channelstore.BroadcastCodecH264
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		if r.log != nil {
			r.log.Debug("codec: channel read failed; defaulting h264", "channel", channelID, "err", err)
		}
		return channelstore.BroadcastCodecH264
	}
	if ch.BroadcastCodec == "" {
		return channelstore.BroadcastCodecH264
	}
	return ch.BroadcastCodec
}

// LineupFormats returns the stream facts of each distinct programme in the channel's cycle, the
// input to playout.DeriveChannelFormats (#1512 G10). The facts are the scanner's read of each
// item's playing file, never probed. An item the Library has not read is the zero MediaFormat,
// which the derivation ignores.
func (r *Resolver) LineupFormats(ctx context.Context, channelID string) ([]playout.MediaFormat, error) {
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, r.now())
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var formats []playout.MediaFormat
	for _, sl := range slots {
		// Filler and pending acquisitions are SDR and file-less; only landed programmes can warrant 4K/HDR.
		if sl.Kind != schedule.SlotProgram || sl.LibraryItemID == "" || seen[sl.LibraryItemID] {
			continue
		}
		seen[sl.LibraryItemID] = true
		format, _ := r.libraryFormat(ctx, sl.LibraryItemID)
		formats = append(formats, format)
	}
	return formats, nil
}

// maxCodecProbes bounds how many program files ComputeChannelCodec probes to decide the majority.
//
// Curation is a human-triggered action, not a hot loop, but a household lineup can still be scores
// of titles and each probe is an ffprobe exec (~tens–hundreds of ms). Sampling the first N program
// slots is enough to establish the majority codec reliably — a channel's library is overwhelmingly
// one codec in practice, and a genuinely mixed one is decided by the same N-sample majority the
// full scan would reach. Past N, the tail would only confirm the lead. Bounds the approve latency.
const maxCodecProbes = 12

// measureChannelCodec is the pure-ish measurement half of ComputeChannelCodec: sample → probe →
// majority, no persistence. Split out so the storing wrapper stays trivial and the sampling policy
// (which slots, how many, tiebreak) lives in one place.
func (r *Resolver) measureChannelCodec(ctx context.Context, channelID string) string {
	if r.probeFormat == nil {
		return channelstore.BroadcastCodecH264 // cannot measure → safe default
	}
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, r.now())
	if err != nil {
		if r.log != nil {
			r.log.Debug("codec: cycle preview failed; defaulting h264", "channel", channelID, "err", err)
		}
		return channelstore.BroadcastCodecH264
	}

	var codecs []string
	probed := 0
	for _, sl := range slots {
		if probed >= maxCodecProbes {
			break
		}
		// Only real, landed PROGRAMS carry a file to probe. Pending acquisitions have no
		// LibraryItemID yet, and filler/flex are normalized TO the channel codec, not sources of it.
		if sl.Kind != schedule.SlotProgram || sl.LibraryItemID == "" {
			continue
		}
		input := r.itemInput(ctx, sl.LibraryItemID)
		if input == "" {
			continue
		}
		fmtInfo, perr := r.probeFormat(ctx, input)
		if perr != nil {
			if r.log != nil {
				r.log.Debug("codec: probe failed; skipping title", "channel", channelID, "item", sl.LibraryItemID, "err", perr)
			}
			continue
		}
		probed++
		codecs = append(codecs, fmtInfo.VideoCodec)
	}
	return majorityBroadcastCodec(codecs)
}

// majorityBroadcastCodec turns a sample of probed video-codec strings into the one broadcast codec
// the channel normalizes to (§9.1 V50 Q2: MAJORITY WINS). Everything non-HEVC counts as h264 —
// h264/mpeg2/vp9/… all normalize DOWN to h264 — so the vote is strictly HEVC vs. not-HEVC.
//
// An even split (including the empty sample: no probes, or an all-pending/filler lineup) breaks to
// h264, the maximally-compatible choice: h264/TS needs no fMP4 and no HEVC decoder, so a channel
// that isn't clearly HEVC-dominant stays playable everywhere. Pure + total, so the sampling policy
// above and this decision are testable in isolation.
func majorityBroadcastCodec(codecs []string) string {
	var hevc, other int
	for _, c := range codecs {
		if playout.IsHEVCCodec(c) {
			hevc++
		} else {
			other++
		}
	}
	if hevc > other {
		return channelstore.BroadcastCodecHEVC
	}
	return channelstore.BroadcastCodecH264
}

// BroadcastsBetween resolves a channel's programme timeline for the XMLTV guide (§9.1, V6b).
//
// Deliberately on the SAME type as AiringNow. The rolling window on air reads the SAME cycle as
// the encoder (airingCycle); only other windows use CyclePreview as a forecast.
//
// `at` is the window's START, not `now`. CyclePreview evaluates curation rules at an instant —
// a rule that switches the channel to horror at 21:00 changes the lineup — and a guide built at
// `now` would advertise the current rule's lineup for the whole window. Using `from` means the
// listings reflect what the rules said when the window opened; a window spanning a rule
// boundary is a known limitation rather than a silent wrong answer (the mid-window portion
// shows the earlier rule's programmes).
//
// cycleAt is CyclePreview with the arranged cycle memoised (cyclecache.go). It is always a
// FORECAST: the window on air comes from airingCycle, which segmentedBroadcasts reads first.
//
// GUIDE PATHS ONLY. AiringNow deliberately does not use this: it is what ffmpeg streams, and
// §9.1's one-source rule is worth more on the broadcast path than the milliseconds a cache would
// save there (one channel, once per programme, versus every channel on every grid poll).
//
// Degrades to the live computation whenever the cache cannot be trusted: no store, no cache, or
// a channel that will not load or fingerprint. Every one of those returns the same answer
// CyclePreview would — only slower — so a cache failure is a performance regression, never a
// wrong lineup.
// Returns the arranged slots AND the resolved rolling-window horizon, which segmentedBroadcasts
// needs to know where the deck rotates. The window is part of the same CyclePreview answer, so
// carrying it costs nothing and saves the caller re-deriving the rule > channel > default
// precedence (and getting it subtly different).
func (r *Resolver) cycleAt(
	ctx context.Context, channelID string, at time.Time,
) ([]schedule.Slot, time.Duration, error) {
	if r.cycles == nil || r.channels == nil {
		_, slots, _, window, err := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, err
	}

	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		// The engine loads the channel itself and will surface the real error (or succeed, if
		// this was a transient read) — so fall through rather than failing the row here.
		_, slots, _, window, cerr := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, cerr
	}

	// The key carries what `at` changes about the arrangement (active rule, active holidays; the
	// rolling-window index is added inside the cache), not the wall-clock — so a navigated window
	// or a later visit reuses the work. See the cycle cache's header.
	_, zone := r.engine.RollingWindow(ch.Policy, at)
	key, ok := fingerprintChannel(channelID, ch.Lineup, ch.Policy, schedule.ClockSignature(ch.Policy, at, r.engine.Calendar()))
	if !ok {
		_, slots, _, window, cerr := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, cerr
	}

	if slots, window, hit := r.cycles.get(key, at, zone); hit {
		return slots, window, nil
	}

	_, slots, _, window, err := r.engine.CyclePreview(ctx, channelID, at)
	if err != nil {
		return nil, 0, err
	}
	r.cycles.put(key, at, zone, slots, window)
	return slots, window, nil
}

// maxGuideSegments bounds how many rolling windows one guide request will re-resolve.
//
// The FE offers a 7-day forward span (RETENTION_DAYS + FORWARD_DAYS) against a 24h default
// window, so 8 covers the whole picker with a segment to spare. The cap is a backstop against a
// pathological window (a channel configured to a one-minute horizon must not turn one guide
// request into thousands of arrangements), not an expected limit: past it the tail of the span
// keeps the last segment's cycle.
const maxGuideSegments = 8

// segmentedBroadcasts walks [from, to) one ROLLING WINDOW at a time, re-resolving the channel's
// cycle at each turn, and concatenates the results. The window on air is the encoder's own
// (airingCycle). With carry-over (#1675) a window turns where the programme crossing its boundary
// ends, not on the boundary itself (carryOverLegs).
//
// # Why this exists
//
// The scheduler ROTATES: windowSliceDealt advances its start by the window index, so day 0 airs one
// slice of the deck and day 1 continues where it left off (§6.5, "over a full cycle every program
// airs"). Reconciliation commits that rotation into Desired at the new window; AiringNow reads the
// accepted value. A multi-day guide still has to forecast the later windows: resolving ONE cycle
// at `from` and looping it across the whole requested span would advertise a rotation that will
// never happen.
//
// Measured on a 27-title channel at the same instant 24h out: the guide advertised RoboCop / The
// Running Man / The Empire Strikes Back while the scheduler would arrange Lethal Weapon / Lethal
// Weapon 2 / Akira. Not merely under-reporting variety — a wrong EPG.
//
// (The XMLTV document escaped this: it publishes a 24h window, which is one rotation step, so it
// was right by construction. Loomarr's own grid queries up to 7 days, which is where it showed.)
//
// # Why here and not inside playout.BroadcastsBetween
//
// That walk is SHARED with the encoder and is deliberately a pure function of one slot list — it
// is what guarantees the grid and the encoder cannot disagree about what is on at 21:00. Teaching
// it to re-resolve would give it a dependency on the engine and put the invariant at risk. The
// segmentation belongs to the caller that spans multiple windows; each segment still goes through
// the identical shared walk.
//
// A window of 0 (WindowFull / unbounded) means the deck never rotates, so one segment is the
// whole answer.
func (r *Resolver) segmentedBroadcasts(
	ctx context.Context, channelID string, from, to time.Time,
	project func(slots []schedule.Slot, epoch, segFrom, segTo time.Time) []playout.Broadcast,
) ([]playout.Broadcast, error) {
	if r.channels == nil {
		return nil, errors.New("read channel playout anchor: channel reader is not configured")
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("read channel playout anchor %s: %w", channelID, err)
	}
	// Reconcile stamps the anchor when the channel's first playable deck goes live, so a channel
	// that has never been live has no timeline yet: an empty row, not corrupt state (#1630, which
	// logged a failure for it on every guide read). Live and drifted keep the refusal below.
	if ch.PlayoutAnchor.IsZero() && ch.Status != schedule.StatusLive && ch.Status != schedule.StatusDrifted {
		return nil, nil
	}
	live, err := r.airingCycle(ctx, ch, r.now())
	if err != nil {
		return nil, err
	}
	if live.slots == nil { // nothing accepted yet: forecast the window on air, from its opening
		now := r.now()
		if live.slots, _, err = r.cycleAt(ctx, channelID, now); err != nil {
			return nil, err
		}
		if live.carriesOver && live.window > 0 {
			live.epoch = later(live.epoch, schedule.WindowStart(now, live.window, live.zone))
		}
	}
	// Unbounded window ⇒ no rotation ⇒ nothing to segment.
	if live.window <= 0 {
		return project(live.slots, live.epoch, from, to), nil
	}

	var legs []guideLeg
	if live.carriesOver {
		legs = r.carryOverLegs(ctx, channelID, live, from, to)
	} else {
		legs = r.fixedAnchorLegs(ctx, channelID, live, from, to)
	}
	out := make([]playout.Broadcast, 0, 32)
	for _, leg := range legs {
		segFrom, segTo := later(from, leg.from), earlier(to, leg.to)
		if !segTo.After(segFrom) {
			continue
		}
		// Project keeps the true start/stop of a programme crossing the requested viewport.
		// That is correct at the OUTER Guide edges, but an INTERNAL leg edge changes which
		// arranged cycle is authoritative. Clip there or the outgoing cycle's tail and the
		// incoming cycle's head occupy the same wall-clock time. A carry-over edge is a programme
		// boundary, so the clip is a no-op there; it cuts only where the channel really cuts.
		clipFrom, clipTo := time.Time{}, time.Time{}
		if leg.from.After(from) {
			clipFrom = leg.from
		}
		if leg.to.Before(to) {
			clipTo = leg.to
		}
		for _, broadcast := range project(leg.slots, leg.epoch, segFrom, segTo) {
			if clipped, ok := broadcast.ClipTo(clipFrom, clipTo); ok {
				out = append(out, clipped)
			}
		}
	}
	return out, nil
}

// guideLeg is one stretch of a channel's timeline: `slots` walked from `epoch`, airing over
// [from, to).
type guideLeg struct {
	slots    []schedule.Slot
	epoch    time.Time
	from, to time.Time
}

// carryOverLegs lays out the timeline of a channel whose rolling window turns with carry-over
// (#1675): the window on air runs until the programme crossing its boundary ends, and each later
// window is forecast from where the one before it ends, through the same turn rule reconcile
// commits (playout.CarryOverEnd). No programme is cut at a boundary and none is joined part-way.
//
// Windows BEFORE the one on air are no longer in state (each turn moved the anchor), so each is
// walked from its own opening, the closest estimate of where it began, and the last runs to where
// the window on air began. The past is a reconstruction either way: it is recomputed from the
// current lineup, which is why the guide's lookback is bounded.
//
// One segment failing must not empty the guide — keep what we have. The grid renders a shorter
// forecast rather than nothing, the same posture the per-channel failure takes in
// api.channelGuide.
func (r *Resolver) carryOverLegs(ctx context.Context, channelID string, live airingCycle, from, to time.Time) []guideLeg {
	var legs []guideLeg
	liveOpened := schedule.WindowStart(live.epoch, live.window, live.zone)
	// A `from` between the live window's opening and its anchor still needs the window before:
	// that stretch is its carried-over programme.
	first := schedule.WindowStart(earlier(from, liveOpened.Add(-time.Nanosecond)), live.window, live.zone)
	for opened := first; from.Before(live.epoch) && opened.Before(liveOpened) && opened.Before(to) && len(legs) < maxGuideSegments; {
		next := schedule.NextWindowStart(opened, live.window, live.zone)
		slots, _, err := r.cycleAt(ctx, channelID, opened)
		if err != nil {
			break
		}
		leg := guideLeg{slots: slots, epoch: opened, from: opened, to: next}
		if !next.Before(liveOpened) {
			leg.to = live.epoch
		}
		legs = append(legs, leg)
		opened = next
	}

	leg := guideLeg{slots: live.slots, epoch: live.epoch, from: live.epoch}
	for {
		leg.to = playout.CarryOverEnd(leg.slots, leg.epoch, schedule.NextWindowStart(leg.epoch, live.window, live.zone))
		legs = append(legs, leg)
		if !leg.to.Before(to) || len(legs) >= maxGuideSegments {
			break
		}
		slots, _, err := r.cycleAt(ctx, channelID, leg.to)
		if err != nil {
			break
		}
		leg = guideLeg{slots: slots, epoch: leg.to, from: leg.to}
	}
	return legs
}

// fixedAnchorLegs lays out the timeline of a channel that cannot carry over (Tunarr-backed; see
// CarriesOver): every rolling window is walked from the channel's one fixed anchor and cut at the
// window's edges, because that is where the backend swaps the list.
func (r *Resolver) fixedAnchorLegs(ctx context.Context, channelID string, live airingCycle, from, to time.Time) []guideLeg {
	var legs []guideLeg
	onAir := schedule.WindowStart(r.now(), live.window, live.zone)
	for segFrom := from; segFrom.Before(to) && len(legs) < maxGuideSegments; {
		next := schedule.NextWindowStart(segFrom, live.window, live.zone)
		slots := live.slots
		if !schedule.WindowStart(segFrom, live.window, live.zone).Equal(onAir) {
			var err error
			if slots, _, err = r.cycleAt(ctx, channelID, segFrom); err != nil {
				break
			}
		}
		legs = append(legs, guideLeg{slots: slots, epoch: live.epoch, from: segFrom, to: next})
		segFrom = next
	}
	return legs
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (r *Resolver) BroadcastsBetween(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	bs, err := r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsBetween)
	if err != nil {
		return nil, err
	}
	r.attachMetadata(ctx, bs)
	return bs, nil
}

// BroadcastsWithPending is BroadcastsBetween plus pending acquisitions, for the time grid
// (V13b). Same CyclePreview, same epoch, same arithmetic — only the projection differs, so the
// grid cannot disagree with the encoder about what airs when.
func (r *Resolver) BroadcastsWithPending(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	bs, err := r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsWithPending)
	if err != nil {
		return nil, err
	}
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
func (r *Resolver) attachProvenance(ctx context.Context, bs []playout.Broadcast) {
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

// attachMetadata fills in descriptions, genres, years and ratings from the Library's tables, in
// ONE read for the whole channel (#2151 step 16).
//
// BEST-EFFORT BY DESIGN. A failure leaves the broadcasts exactly as they were — titles and times
// intact — because a guide with thin entries is far better than no guide.
func (r *Resolver) attachMetadata(ctx context.Context, bs []playout.Broadcast) {
	if r.itemFacts == nil || len(bs) == 0 {
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

	meta, err := ItemMetadata{Facts: r.itemFacts}.ItemMetadataByID(ctx, ids)
	if err != nil && r.log != nil {
		// Logged, not returned: the partial map is still applied below, so a failing read does
		// not cost the whole document its descriptions.
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
func (r *Resolver) airingFiller(
	ctx context.Context, channelID string, gap playout.Airing, now time.Time,
) (playout.Airing, string, error) {
	if r.pods == nil || r.fillerDir == "" {
		// Filler unconfigured: the break becomes a bounded break card rather than an error. Keep
		// the gap itself so the handler knows this is scheduled airtime and where it ends.
		return gap, "", nil
	}

	// PreviewAt, not Preview: the pod is seeded from THIS break's start, so consecutive
	// breaks play different adverts — and the guide's hover card, which calls the same
	// method with the same start, lists exactly what will air here.
	//
	// gap.Offset is how far into the break we are, so the break began that long ago, and a break is
	// assembled to its own slot, never past it (a placeholder to the configured break length).
	pod, err := r.pods.PreviewAt(ctx, channelID, pod.BreakSlot{
		Start: time.UnixMilli(now.Add(-gap.Offset).UnixMilli()), Length: gap.BreakLength(),
	})
	if err != nil || len(pod.Entries) == 0 {
		// No pool, or the assembler could not fill this break. Not an error: §10's ladder
		// bottoms out at "nothing matched", and a channel must not fail to tune because it has
		// no ads.
		return gap, "", nil
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
				return gap, "", nil
			}
			// ⚠ ClipPath is the containment check, not a join: the id comes from the database
			// and a crafted `../` would otherwise stream an arbitrary file off the host.
			full, perr := cliplibrary.ClipPath(r.fillerDir, e.Path, "")
			if perr != nil {
				return gap, "", nil
			}
			// Resolving a clip does not count it: pod ASSEMBLY re-runs on every 10m reconcile
			// sweep and any read of the channel resolves the airing, so counting here would count
			// what was scheduled or looked at, not what aired. RecordAiringServed counts it when
			// a viewer receives its segments.
			remaining := d - into
			if gap.Remaining > 0 && gap.Remaining < remaining {
				remaining = gap.Remaining
			}
			// The loudness measured at ingest rides the airing so the channel packager can apply a
			// static gain instead of a live loudnorm (#1512 G6). Absent ⇒ 0 dB.
			var measured, truePeak *float64
			if lufs, peak, ok := asset.PlaybackLoudness(full); ok {
				measured, truePeak = &lufs, peak
			}
			return playout.Airing{
				StartedAt:       now.Add(-into),
				Identity:        e.Hash,
				ScheduleBlockID: gap.ScheduleBlockID,
				Kind:            schedule.SlotFiller,
				// Source, not LibraryItemID: this is a local file, not a media-server item.
				// Playable() checks Source for exactly this case.
				Source:           full,
				MeasuredLUFS:     measured,
				MeasuredTruePeak: truePeak,
				RecentRepeat:     e.RecentRepeat,
				RotationPinned:   e.RotationPinned,
				Title:            e.Name,
				Offset:           into,
				Remaining:        remaining,
			}, full, nil
		}
		into -= d
	}

	// The pod is shorter than the break gap. Real: a 30s break with 20s of clips. The remainder
	// is the offline card rather than a repeat, because repeating would mean the same ad twice
	// in one break — worse than a moment of card.
	return gap, "", nil
}

// AudioTrackFor resolves which audio track a source should play (§9.1).
//
// Returns an index among AUDIO streams — the `N` in `-map 0:a:N`. Zero means "the file's first
// track", which is the safe answer for every failure below.
//
// ⚠ BEST-EFFORT, NEVER FATAL, and every branch here degrades the same way for the same reason:
// a probe that fails must cost the viewer the language preference, never the programme. No
// preference set, no prober wired, an unreachable media server, an ffprobe that is missing or
// slow — all of them fall through to track 0 and the channel keeps playing. The alternative is a
// channel that goes dark because a metadata read timed out, which trades a small annoyance for a
// total outage.
//
// One probe per PROGRAMME, not per request: the channel packager asks for a new program at each
// boundary, so this runs about as often as a film is long. That is what makes an exec on the
// broadcast path affordable — and why it must not become per-segment.
func (r *Resolver) AudioTrackFor(ctx context.Context, channelID, libraryItemID, streamURL string) int {
	if r.audioLanguage == nil || streamURL == "" {
		return 0
	}
	prefer := r.preferredAudioLanguage(ctx, channelID)
	if strings.TrimSpace(prefer) == "" {
		// Explicitly cleared ⇒ the operator wants ffmpeg's original behaviour. Skip the probe
		// entirely rather than paying for an answer that cannot change the outcome.
		return 0
	}
	if tracks, ok := r.inventoryAudioTracks(ctx, streamURL); ok {
		return playout.PickAudioTrack(tracks, prefer)
	}
	if r.probeSource == nil {
		return 0
	}
	observed, err := r.probeSource(ctx, streamURL)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: audio tracks not probed, using the first",
				"err", err)
		}
		return 0
	}
	tracks := observed.AudioTracks()
	r.recordSourceMeasurement(ctx, streamURL, observed)
	return playout.PickAudioTrack(tracks, prefer)
}

func (r *Resolver) preferredAudioLanguage(ctx context.Context, channelID string) string {
	prefer := r.audioLanguage()
	// A channel override wins over the instance default. Failure to reload the channel is
	// deliberately non-fatal: AiringNow already resolved enough state to play, so falling back
	// to the global preference is better than taking the programme off air for an optional track.
	if r.channels != nil && channelID != "" {
		if ch, err := r.channels.GetChannel(ctx, channelID); err == nil {
			prefer = schedule.ResolveAudioLanguage(ch.Policy, prefer)
		}
	}
	return prefer
}

// LibraryFileReader is the store's read of the file a Library item plays.
type LibraryFileReader interface {
	LibraryPlayingFile(ctx context.Context, itemID string) (string, bool, error)
}

// itemInput is the input ffmpeg reads for a programme: the library item's own playing file, while
// it is readable. Playout reads only files Loomarr's scan found (#2151 step 16); an id the Library
// does not hold, or a file that is gone, has nothing to play: "".
func (r *Resolver) itemInput(ctx context.Context, itemID string) string {
	if r.libraryFile == nil {
		return ""
	}
	path, ok, err := r.libraryFile.LibraryPlayingFile(ctx, itemID)
	if err != nil && r.log != nil {
		r.log.Debug("playout: library file unavailable", "item", itemID, "err", err)
	}
	if !ok || !readableFile(path) {
		return ""
	}
	return path
}

// readableFile reports whether path is a regular file this process can open. A directory, a
// missing path or an unreadable file is false.
func readableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// LibraryVideoReader is the store's read of the video a Library item plays.
type LibraryVideoReader interface {
	LibraryPlayingVideo(ctx context.Context, itemID string) (store.LibraryStream, bool, error)
}

// libraryFormat is a library item's video format as the scanner read its playing file's header
// (#2151 step 15): what a channel's format and a still's tone mapping need. ok is false for any
// other id, which the inventory answers.
func (r *Resolver) libraryFormat(ctx context.Context, itemID string) (playout.MediaFormat, bool) {
	if r.libraryVideo == nil {
		return playout.MediaFormat{}, false
	}
	v, ok, err := r.libraryVideo.LibraryPlayingVideo(ctx, itemID)
	if err != nil || !ok {
		if err != nil && r.log != nil {
			r.log.Debug("playout: library video unavailable", "item", itemID, "err", err)
		}
		return playout.MediaFormat{}, false
	}
	return playoutFormatOf(inventory.SourceFacts{Streams: []inventory.Stream{{
		Index: v.Index, Kind: inventory.StreamVideo, Codec: v.Codec, Width: v.Width, Height: v.Height,
		FrameRate: v.FrameRate, PixelFormat: v.PixelFormat, ColorTransfer: v.ColorTransfer,
	}}}), true
}

// LibraryAudioReader is the store's one Library read audio selection makes.
type LibraryAudioReader interface {
	LibraryAudioTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) ([]store.LibraryStream, bool, error)
}

// libraryAudioTracks is a local file's audio tracks as the Library lists them, when the scanner
// read the file as it is now (#2151 decision 47).
func (r *Resolver) libraryAudioTracks(ctx context.Context, streamURL string) ([]playout.AudioTrack, bool) {
	if r.libraryAudio == nil {
		return nil, false
	}
	_, info, ok := localInventoryOrigin(streamURL)
	if !ok {
		return nil, false
	}
	streams, ok, err := r.libraryAudio.LibraryAudioTracks(ctx, streamURL, info.Size(), info.ModTime().UnixNano())
	if err != nil || !ok {
		return nil, false
	}
	tracks := make([]playout.AudioTrack, 0, len(streams))
	for _, s := range streams {
		tracks = append(tracks, playout.AudioTrack{SourceIndex: s.Index, Language: s.Language})
	}
	return tracks, true
}

// inventoryAudioTracks is a local file's audio tracks as the Library lists them, else as Loomarr
// measured the file's current bytes. A file neither has read falls through to one probe.
func (r *Resolver) inventoryAudioTracks(ctx context.Context, streamURL string) ([]playout.AudioTrack, bool) {
	if tracks, ok := r.libraryAudioTracks(ctx, streamURL); ok {
		return tracks, true
	}
	if r.inventory == nil {
		return nil, false
	}
	if localOrigin, ok := r.ensureLocalInventorySource(ctx, streamURL); ok {
		source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
			Item: inventory.ItemRef{Origin: &localOrigin}, Now: r.inventoryNow(),
			Kinds: []inventory.SourceKind{inventory.SourceLocalFile}, RequiredCoverage: []string{"audioStreams"},
		})
		if err == nil && found {
			return playoutAudioTracks(source.Observation.Facts.Streams), true
		}
	}
	// A local file's stat revision is the authoritative freshness boundary, so a miss falls
	// directly through to one probe.
	return nil, false
}

// recordSourceMeasurement files a probe of a local file under its current bytes, so the next
// programme of the same file reads it back instead of probing again.
func (r *Resolver) recordSourceMeasurement(ctx context.Context, streamURL string, observed playout.SourceObservation) {
	if r.inventory == nil {
		return
	}
	localOrigin, ok := r.ensureLocalInventorySource(ctx, streamURL)
	if !ok {
		return
	}
	source, ok, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{Item: inventory.ItemRef{Origin: &localOrigin}})
	if err != nil || !ok {
		return
	}
	measurement := inventory.NewFactsMeasurement(source.ID, source.Revision, r.inventoryNow(), InventoryFactsOf(observed))
	if err := r.inventory.RecordMeasurement(ctx, measurement); err != nil && r.log != nil {
		r.log.Debug("playout: measured audio observation not persisted", "err", err)
	}
}

// localInventoryOrigin is the inventory identity of a local file: its path's origin key and the
// stat revision of its current bytes.
func localInventoryOrigin(input string) (inventory.OriginKey, os.FileInfo, bool) {
	info, err := os.Stat(input)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return inventory.OriginKey{}, nil, false
	}
	digest := sha256.Sum256([]byte(input))
	return inventory.OriginKey{Authority: "local-playout:v1", ExternalItemID: hex.EncodeToString(digest[:16])}, info, true
}

// knownLocalSource reads, never registers, the inventory source of a local file's current bytes.
func (r *Resolver) knownLocalSource(ctx context.Context, input string) (inventory.Source, bool) {
	if r.inventory == nil {
		return inventory.Source{}, false
	}
	origin, info, ok := localInventoryOrigin(input)
	if !ok {
		return inventory.Source{}, false
	}
	revision := localInventoryRevision(info)
	item, found, err := r.inventory.Item(ctx, inventory.ItemRef{Origin: &origin})
	if err != nil || !found {
		return inventory.Source{}, false
	}
	for _, source := range item.Sources {
		if source.Kind == inventory.SourceLocalFile && source.Revision == revision {
			return source, true
		}
	}
	return inventory.Source{}, false
}

func (r *Resolver) ensureLocalInventorySource(ctx context.Context, input string) (inventory.OriginKey, bool) {
	origin, info, ok := localInventoryOrigin(input)
	if !ok {
		return inventory.OriginKey{}, false
	}
	revision := localInventoryRevision(info)
	if _, known := r.knownLocalSource(ctx, input); known {
		return origin, true
	}
	at := r.inventoryNow()
	_, err := r.inventory.ApplySnapshot(ctx, inventory.Snapshot{
		Origin: origin, Kind: "unknown",
		Observation: inventory.Observation[inventory.ItemFacts]{
			SchemaVersion: 1, ObservedAt: at,
			Coverage: map[string]inventory.Coverage{"sources": inventory.CoveragePresent},
		},
		Sources: []inventory.SourceSnapshot{{
			ExternalSourceID: "file", Kind: inventory.SourceLocalFile,
			Revision: revision,
			Locator:  inventory.Locator{Path: input},
			Observation: inventory.Observation[inventory.SourceFacts]{
				SchemaVersion: 1, ObservedAt: at,
				Coverage: map[string]inventory.Coverage{"sourceStat": inventory.CoveragePresent},
				Facts: inventory.SourceFacts{
					SizeBytes: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(),
				},
			},
		}},
	})
	return origin, err == nil
}

func localInventoryRevision(info os.FileInfo) string {
	return fmt.Sprintf("stat:%d:%d", info.Size(), info.ModTime().UnixNano())
}

func InventoryFactsOf(observed playout.SourceObservation) inventory.SourceFacts {
	facts := inventory.SourceFacts{
		Container: observed.Container, DurationMillis: observed.DurationMillis, Bitrate: observed.Bitrate,
		UnsafePreroll: observed.UnsafePreroll,
	}
	for _, stream := range observed.Streams {
		facts.Streams = append(facts.Streams, inventory.Stream{
			Index: stream.Index, Kind: inventory.StreamKind(stream.Kind), Codec: stream.Codec,
			Profile: stream.Profile, Level: stream.Level, Language: stream.Language, Title: stream.Title,
			Disposition: inventory.Disposition{Default: stream.Default, Forced: stream.Forced, Comment: stream.Comment,
				HearingImpaired: stream.HearingImpaired, VisualImpaired: stream.VisualImpaired},
			Channels: stream.Channels, ChannelLayout: stream.ChannelLayout, SampleRate: stream.SampleRate,
			Width: stream.Width, Height: stream.Height, FrameRate: stream.FrameRate,
			PixelFormat: stream.PixelFormat, ColorSpace: stream.ColorSpace,
			ColorTransfer: stream.ColorTransfer, ColorPrimaries: stream.ColorPrimaries,
			HDR: stream.HDR, DolbyVision: stream.DolbyVision, Interlaced: stream.Interlaced,
			SubtitleFormat: func() string {
				if stream.Kind == string(inventory.StreamSubtitle) {
					return stream.Codec
				}
				return ""
			}(),
		})
	}
	return facts
}

func playoutFormatOf(facts inventory.SourceFacts) playout.MediaFormat {
	format := playout.MediaFormat{
		Container:    strings.ToLower(strings.TrimSpace(facts.Container)),
		Duration:     float64(facts.DurationMillis) / float64(time.Second/time.Millisecond),
		Bitrate:      facts.Bitrate,
		VideoPreroll: facts.UnsafePreroll,
	}
	for _, stream := range facts.Streams {
		switch stream.Kind {
		case inventory.StreamVideo:
			if format.VideoCodec != "" {
				continue
			}
			format.VideoCodec = strings.ToLower(strings.TrimSpace(stream.Codec))
			format.Width = stream.Width
			format.Height = stream.Height
			format.FrameRate = playout.ParseFrameRate(stream.FrameRate)
			format.PixelFormat = strings.ToLower(strings.TrimSpace(stream.PixelFormat))
			format.ColorTransfer = strings.ToLower(strings.TrimSpace(stream.ColorTransfer))
			format.Interlaced = stream.Interlaced
		case inventory.StreamAudio:
			if format.AudioCodec != "" {
				continue
			}
			format.AudioCodec = strings.ToLower(strings.TrimSpace(stream.Codec))
			format.AudioChannels = stream.Channels
			format.AudioSampleRate = stream.SampleRate
		}
	}
	return format
}

func (r *Resolver) inventoryNow() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func playoutAudioTracks(streams []inventory.Stream) []playout.AudioTrack {
	tracks := make([]playout.AudioTrack, 0)
	for _, stream := range streams {
		if stream.Kind == inventory.StreamAudio {
			tracks = append(tracks, playout.AudioTrack{SourceIndex: stream.Index, Language: stream.Language})
		}
	}
	return tracks
}

// Tracks probes the audio + subtitle tracks of what's airing on a channel right now (§9.1 Watch,
// V46). It resolves the current program's source URL exactly as the broadcast path does, then
// probes it.
//
// BEST-EFFORT, like AudioTrackFor: nothing airing, no prober wired, or a failed probe all return
// empty tracks rather than an error, so the Watch pickers degrade to "just the channel default"
// instead of erroring. The one thing it does NOT swallow is a genuine store/resolve error (a
// missing channel), which the handler needs to render a 404 rather than an empty list.
func (r *Resolver) Tracks(ctx context.Context, channelID string) (playout.MediaTracks, error) {
	airing, streamURL, err := r.AiringNow(ctx, channelID)
	if err != nil {
		return playout.MediaTracks{}, err
	}
	if r.probeTracks == nil || !airing.Playable() || streamURL == "" {
		return playout.MediaTracks{}, nil
	}
	tracks, err := r.probeTracks(ctx, streamURL)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: tracks not probed for Watch pickers", "channel", channelID, "err", err)
		}
		return playout.MediaTracks{}, nil // degrade, don't fail the UI request
	}
	return tracks, nil
}

// FormatOf probes an input once and returns everything the probe measured: geometry, framerate,
// pixel format, colour transfer, audio shape, duration, bitrate (playout.MediaFormat).
//
// BEST-EFFORT, and it fails SAFE: no prober wired, no input, or a probe error all return the zero
// MediaFormat, which reads as SDR/unknown rather than as anything asserted, so the item transcodes.
// A copy of an unprobed source could ship a codec the output cannot play — a black frame — whereas
// an unnecessary transcode is merely slow.
//
// A library item's colour transfer comes from the scanner's header read of its playing file when
// that names one (#2173): an HDR10 Matroska file can signal PQ only in the HEVC parameter set, which
// the header reader parses and ffprobe's stream summary reports as unknown, so the airing would skip
// tone mapping. Any other id keeps the probe's.
func (r *Resolver) FormatOf(ctx context.Context, libraryItemID, input string) playout.MediaFormat {
	format := r.probedFormat(ctx, input)
	if libraryItemID == "" {
		return format
	}
	if header, ok := r.libraryFormat(ctx, libraryItemID); ok && header.ColorTransfer != "" && header.ColorTransfer != "unknown" {
		format.ColorTransfer = header.ColorTransfer
	}
	return format
}

func (r *Resolver) probedFormat(ctx context.Context, input string) playout.MediaFormat {
	if input == "" {
		return playout.MediaFormat{}
	}
	if r.inventory != nil {
		if origin, ok := r.ensureLocalInventorySource(ctx, input); ok {
			source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
				Item: inventory.ItemRef{Origin: &origin}, Now: r.inventoryNow(),
				Kinds: []inventory.SourceKind{inventory.SourceLocalFile}, RequiredCoverage: []string{"streams"},
			})
			if err == nil && found {
				format := playoutFormatOf(source.Observation.Facts)
				format.Active = r.activePicture(ctx, source.ID)
				r.submitAnalysis(source.ID, source.Revision, input, source.Observation.Facts)
				return format
			}
			if format, ok := r.measureFirstPlay(ctx, origin, input); ok {
				return format
			}
		}
	}
	if r.probeFormat == nil {
		return playout.MediaFormat{}
	}
	f, err := r.probeFormat(ctx, input)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: format not probed, transcoding", "input", input, "err", err)
		}
		return playout.MediaFormat{}
	}
	return f
}

// Profile is the encode profile at the ladder rung the ResourceBudget admitted a session at,
// resolved against the measured encoder and the operator's quality tier (§9.1: "best picture the
// hardware sustains, then adapt"). A new session takes the best rung that fits and keeps it.
func (r *Resolver) Profile(ctx context.Context, rung int) playout.Profile {
	enc := encode.Encoder(r.encoder())
	if enc == "" {
		// A matching persisted result is available to this first tune after only its host fingerprint
		// matches. Its real encoder validation runs asynchronously; a miss likewise starts the full
		// benchmark in the background and uses software until a safe result is ready.
		//
		// Falling straight through to libx264 would leave a box with a working GPU silently
		// encoding in software forever. Detect trial-encodes each candidate, so its answer is
		// measured rather than inferred from `ffmpeg -encoders` (which lists encoders the
		// hardware cannot actually run — the exact trap that took a live channel down: the
		// host listed h264_vulkan, the container had no /dev/dri).
		r.reuseEncoderEvidence(ctx)
		enc = encode.EncoderSoftware
		if r.detectReady.Load() {
			enc = r.PublishedEncoder()
		}
		r.warmEncoderDetection(ctx)
	}
	return playout.Resolve(playout.TierFor(r.tier()), enc, rung)
}

// WarmProfile loads the host's persisted encoder evidence ahead of the first tune (#1512 G2: its
// host fingerprint was 0.2 s of a cold first tune). It is cheap (one `ffmpeg -version`); the full
// encoder measurement still starts only on the first demand.
func (r *Resolver) WarmProfile(ctx context.Context) { r.reuseEncoderEvidence(ctx) }

func (r *Resolver) reuseEncoderEvidence(ctx context.Context) {
	r.detectEvidence.Do(func() {
		var capacity playout.Capacity
		var ok bool
		if r.loadCapabilityEvidence != nil {
			capacity, ok = r.loadCapabilityEvidence(ctx)
		} else {
			bin, gpu, root := r.capabilityInputs()
			capacity, ok = playout.LoadMatchingObservedCapabilityEvidence(
				ctx, bin, playout.DefaultProfile(), gpu, root,
			)
		}
		if ok {
			r.publishDetectedCapacity(capacity, true)
		}
	})
}

// warmEncoderDetection starts the expensive host probe once without putting it on a viewer's
// critical path. The composition root's lifetime wins over an individual request context so one
// TV disconnecting cannot discard the result every later tune needs.
func (r *Resolver) warmEncoderDetection(fallback context.Context) {
	r.detectStart.Do(func() {
		ctx := r.detectContext
		if ctx == nil {
			ctx = context.WithoutCancel(fallback)
		}
		go r.detectedEncoder(ctx)
	})
}

// detectedEncoder returns the best encoder that actually WORKS here, probing once. Control-plane
// callers may wait for it; Profile deliberately uses warmEncoderDetection + detectReady instead.
//
// Lazily and exactly once per process, which is the only workable timing: a full Detect trials
// every candidate and then measures the winner warm. The first demand starts it asynchronously;
// everything after completion reads the result. A matching persisted hardware result substitutes
// one short real TS encode, while an FFmpeg/GPU/profile mismatch or failed validation performs the
// complete measurement and atomically replaces that evidence.
func (r *Resolver) detectedEncoder(ctx context.Context) encode.Encoder {
	r.detectOnce.Do(func() {
		bin, gpu, root := r.capabilityInputs()
		cap, reused := playout.DetectObservedWithEvidence(
			ctx, bin, playout.DefaultProfile(), gpu, root, r.processDiagnostics,
		)
		r.publishDetectedCapacity(cap, reused)
	})
	return r.PublishedEncoder()
}

func (r *Resolver) capabilityInputs() (bin, gpu, root string) {
	r.inputsOnce.Do(func() {
		if r.ffmpegPath != nil {
			r.capabilityBin = r.ffmpegPath()
		}
		if r.capabilityBin == "" {
			r.capabilityBin = "ffmpeg"
		}
		if r.gpuName != nil {
			r.capabilityGPU = r.gpuName()
		}
		if r.capabilityRoot != nil {
			r.capabilityPath = r.capabilityRoot()
		}
	})
	return r.capabilityBin, r.capabilityGPU, r.capabilityPath
}

func (r *Resolver) publishDetectedCapacity(cap playout.Capacity, reused bool) {
	r.detectedMu.Lock()
	r.detected = cap.Chosen
	r.detectedMu.Unlock()
	r.maxChannels.Store(int64(cap.MaxChannels))
	if cap.EncodeHostBytes > 0 {
		// Only a real measurement replaces the previous one; a cheap evidence reload carries none.
		r.encodeHostBytes.Store(cap.EncodeHostBytes)
	}
	if r.log != nil {
		skipped := make([]string, 0, len(cap.All))
		for _, c := range cap.All {
			if !c.Works {
				skipped = append(skipped, string(c.Encoder)+": "+c.Err)
			}
		}
		r.log.Info("playout: encoder probed",
			"chosen", cap.Chosen, "measured_max_channels", cap.MaxChannels,
			"reused_verified_evidence", reused, "skipped", skipped)
	}
	r.detectReady.Store(true)
}

func (r *Resolver) PublishedEncoder() encode.Encoder {
	r.detectedMu.RLock()
	defer r.detectedMu.RUnlock()
	return r.detected
}

// FileEncodeHost is the hardware filler derivatives encode on (#2121), the channels' own: the
// operator's pinned encoder, else the one detection published, and software until detection
// settles. encode.FileFamily then keeps a family without a measured level in software.
func (r *Resolver) FileEncodeHost(pinned encode.Encoder) encode.HostProfile {
	enc := pinned
	if enc == "" && r != nil && r.detectReady.Load() {
		enc = r.PublishedEncoder()
	}
	if enc == "" {
		enc = encode.EncoderSoftware
	}
	return encode.HostFor(enc, false, encode.GPUFilters{})
}

// PublishedCapability returns only the encoder result already published by the asynchronous
// capability probe. Dashboard polling must never start or wait for a benchmark.
func (r *Resolver) PublishedCapability() playout.Capacity {
	if !r.detectReady.Load() {
		return playout.Capacity{}
	}
	return playout.Capacity{Chosen: r.PublishedEncoder(), MaxChannels: int(r.maxChannels.Load())}
}

// EncodeHostBytes is the measured host memory one hardware encode holds, 0 until a trial encode
// has measured it on this host.
func (r *Resolver) EncodeHostBytes() int64 {
	return r.encodeHostBytes.Load()
}

// effectivePlayoutAnchor enforces the persisted timeline origin. A live channel without
// one is corrupt control-plane state; guessing would let the encoder and guide diverge.
func effectivePlayoutAnchor(ch channelstore.Channel) (time.Time, error) {
	if ch.PlayoutAnchor.IsZero() {
		return time.Time{}, fmt.Errorf("channel %s has no playout anchor", ch.ID)
	}
	return ch.PlayoutAnchor, nil
}
