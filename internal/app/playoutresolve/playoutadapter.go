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

	"errors"
	"fmt"
	"log/slog"

	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
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
	// librarySubtitles and subtitleCues serve a channel's soft subtitles (#2088 decision 5); nil
	// until SetSubtitles, which leaves every window empty.
	librarySubtitles LibrarySubtitleReader
	subtitleCues     SubtitleCues
	// itemFacts answers the guide's metadata for a library item id from the Library's tables; nil
	// leaves the guide without descriptions, and any other id has none.
	itemFacts librarystore.LibraryItemFactsReader
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
	goDetect       func(func(context.Context)) bool
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
	// GoDetect starts the background encoder detection as work its owner joins at shutdown, on a
	// context the owner cancels then; it reports false, and starts nothing, once the owner is
	// closing. Nil runs it unowned on the requesting context, detached from its cancellation.
	GoDetect func(run func(context.Context)) bool
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
	ItemFacts librarystore.LibraryItemFactsReader

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
		goDetect:           cfg.GoDetect,
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
	input := r.programmeInput(ctx, airing.LibraryItemID, airing.Offset)
	if input.Path == "" {
		// The lineup names an item whose file is gone or unreadable, or an id the Library does
		// not hold. Reporting it as "nothing airing" rather than an error means the channel shows
		// the offline card instead of failing to tune — the same outcome the viewer would get
		// from an error, minus the retry storm.
		return playout.Airing{Kind: schedule.SlotFlex}, "", nil
	}
	// A movie in parts airs a part at a time: the encode ends where the part does, and the
	// packager, asking again there, gets the next part from its start.
	airing.PartStart = input.Start
	if input.End > 0 {
		airing.Remaining = min(airing.Remaining, input.End-airing.Offset)
	}
	return airing, input.Path, nil
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
			input := r.programmeInput(ctx, itemID, airing.Offset)
			if input.Path == "" {
				return playout.StillSource{}, false, nil
			}
			source := playout.StillSource{Input: input.Path, HDR: r.inventoryHDR(ctx, itemID), PartStart: input.Start}
			source.Keyframe, source.Indexed = r.indexedKeyframe(ctx, input.Path, airing.Offset-input.Start)
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

// effectivePlayoutAnchor enforces the persisted timeline origin. A live channel without
// one is corrupt control-plane state; guessing would let the encoder and guide diverge.
func effectivePlayoutAnchor(ch channelstore.Channel) (time.Time, error) {
	if ch.PlayoutAnchor.IsZero() {
		return time.Time{}, fmt.Errorf("channel %s has no playout anchor", ch.ID)
	}
	return ch.PlayoutAnchor, nil
}
