package playout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Delivery is the transport shape requested by a playout adapter.
type Delivery uint8

const (
	DeliveryMPEGTS Delivery = iota + 1
	DeliveryHLS
)

var (
	ErrUnsupportedDelivery = errors.New("playout: unsupported delivery")
	// ErrUnavailable is returned while a Postgres replica cannot prove it has observed every
	// committed lifecycle transition. The composition root closes this gate before tearing down
	// local sessions and reopens it only after a durable catch-up.
	ErrUnavailable = errors.New("playout: lifecycle state unavailable")
	// ErrIneligible is the clean lifecycle miss for a channel outside the transport-published
	// internal catalog (paused, detached, empty, or effectively Tunarr-backed).
	ErrIneligible = errors.New("playout: channel is not eligible for internal transport")
)

// TuneRequest is everything a transport adapter must prove to tune a Channel. It deliberately
// carries no encoder, cache, or scratch-directory choice; those are Origin implementation details.
type TuneRequest struct {
	ChannelID string
	Plan      EncodePlan
	Delivery  Delivery
	// Speculative permits bounded live fallback but must not reclaim another Channel's retained
	// session. Adjacent Watch warming uses it so optional work cannot displace foreground playback.
	Speculative bool
}

type viewerContextKey struct{}

// WithViewer names the viewer a playout request is for: an opaque key that is stable for one person
// on one device (the API's signed viewer tag). Playout uses it only to learn what that viewer's
// client plays (#1037), so a request that names nobody is served exactly the same.
func WithViewer(ctx context.Context, viewer string) context.Context {
	if viewer == "" {
		return ctx
	}
	return context.WithValue(ctx, viewerContextKey{}, viewer)
}

// ViewerFrom is the viewer WithViewer named, or "".
func ViewerFrom(ctx context.Context) string {
	viewer, _ := ctx.Value(viewerContextKey{}).(string)
	return viewer
}

// Presentation is one tuned Channel. Exactly one of Stream or Manifest is populated according
// to the requested Delivery. Release must be called when the caller is finished with this snapshot.
type Presentation struct {
	Stream   Stream
	Manifest []byte
	Release  func()
}

// Stream is an ordered raw transport with cancellable reads. Next returns an owned
// chunk or an error; io.EOF follows the last accepted byte. Release the associated
// Presentation when finished, including after cancellation or EOF.
type Stream interface {
	Next(context.Context) ([]byte, error)
}

// hlsPlaylistLease owns one viewer reference while media readiness is pending.
// Acquiring the lease is short and lifecycle-ordered; reading it may wait for media.
type hlsPlaylistLease struct {
	path     string
	release  func()
	await    func(context.Context) error
	snapshot func(context.Context) ([]byte, error)
}

func (l hlsPlaylistLease) readManifest(ctx context.Context) ([]byte, func(), error) {
	if err := l.await(ctx); err != nil {
		l.release()
		return nil, nil, err
	}
	var (
		body []byte
		err  error
	)
	if l.snapshot != nil {
		body, err = l.snapshot(ctx)
	} else {
		body, err = os.ReadFile(l.path)
	}
	if err != nil {
		l.release()
		return nil, nil, err
	}
	return body, l.release, nil
}

type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// Asset is an opened follow-up resource. Callers know its bytes and modification time, never the
// packager's filesystem layout.
type Asset struct {
	Content  readSeekCloser
	Modified time.Time
	// Playlist marks a live media playlist (a packager variant, #1512 phase 2b): its URIs are bare
	// asset names the transport must make self-authenticating, as it does for the Tune manifest.
	Playlist bool
}

type sessionAttacher interface {
	Attach(context.Context, string, EncodePlan) (Stream, func(), error)
	StopChannel(channelID string)
	Stop()
}

type hlsOrigin interface {
	// acquirePlaylist(channel, plan, speculative, viewer)
	acquirePlaylist(string, EncodePlan, bool, string) (hlsPlaylistLease, error)
	AssetPath(string, EncodePlan, string) (string, bool)
	StopChannel(channelID string)
	StopAll()
}

// Origin is the one playout seam used by transport adapters: the channel packager serves every
// delivery behind it (#1512 phase 2), and callers never see its layout.
type Origin struct {
	sessions sessionAttacher
	hls      hlsOrigin

	// lifecycleMu orders admission plus attachment against fail-closed StopAll. The atomic
	// availability callback alone is insufficient: a tune could observe true, then attach after
	// StopAll had already snapshotted the managers and escape teardown.
	lifecycleMu sync.RWMutex
	closed      bool
	available   func() bool
	eligible    func(context.Context, string) (bool, error)

	// Channel stills (still.go): the segment holders, tried in Tune's order, and the decoder.
	stillSources   []stillSource
	stillExtractor StillExtractor
	stillClock     func() time.Time // nil = time.Now; a test seam for the stale-fallback window
	stills         stillCache
}

// OriginDependencies are the implementations hidden behind the one production playout seam.
type OriginDependencies struct {
	// Packager is live playout (#1512 phase 2): one channel packager per watched channel serves
	// the browser's HLS and every media-server tuner's MPEG-TS. Nil leaves live playout off.
	Packager *PackagerHLS
	// Available is a fail-closed admission gate. Nil means always available (the SQLite
	// single-replica path); Postgres supplies a gate tied to its durable invalidation listener.
	Available func() bool
	// Eligible revalidates one channel at the playout seam. Postgres supplies a durable read so
	// an HTTP request admitted just before a remote commit cannot attach after that commit's stop.
	// Nil preserves the SQLite single-replica path's existing local lifecycle behavior.
	Eligible func(context.Context, string) (bool, error)
	// Still decodes one frame of a warm channel's live segment for the channel-switch overlay. Nil
	// disables segment stills — used where no ffmpeg is wired.
	Still StillExtractor
	// StillAiring and SourceStill give a cold channel its still: one frame of the airing on now,
	// decoded from the source on demand. Either nil disables source stills.
	StillAiring StillAiringResolver
	SourceStill SourceStillExtractor
}

// NewOrigin assembles live playout behind one seam.
func NewOrigin(deps OriginDependencies) *Origin {
	// A nil concrete pointer stored directly in an interface is non-nil. Normalize every optional
	// implementation here so degraded construction cannot call through a typed-nil dependency.
	var sessions sessionAttacher
	var hls hlsOrigin
	if deps.Packager != nil {
		sessions, hls = packagedTuners{deps.Packager}, deps.Packager
	}
	o := newOrigin(sessions, hls)
	o.available = deps.Available
	o.eligible = deps.Eligible
	o.stillExtractor = deps.Still
	// A warm channel's newest segment first (fresher, and already decoded media), then the cold
	// channel's source file.
	if deps.Packager != nil {
		o.stillSources = append(o.stillSources, deps.Packager)
	}
	if deps.StillAiring != nil && deps.SourceStill != nil {
		o.stillSources = append(o.stillSources, airingStillSource{resolve: deps.StillAiring, extract: deps.SourceStill})
	}
	return o
}

func newOrigin(sessions sessionAttacher, hls hlsOrigin) *Origin {
	return &Origin{sessions: sessions, hls: hls}
}

func (o *Origin) checkAdmissionLocked(ctx context.Context, channelID string) error {
	if o.closed {
		return ErrUnavailable
	}
	if o.available != nil && !o.available() {
		return ErrUnavailable
	}
	if o.eligible == nil {
		return nil
	}
	eligible, err := o.eligible(ctx, channelID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !eligible {
		return ErrIneligible
	}
	return nil
}

// Tune returns the presentation for the requested delivery without exposing which implementation
// produced it.
func (o *Origin) Tune(ctx context.Context, request TuneRequest) (Presentation, error) {
	presentation, lease, err := o.acquireTune(ctx, request)
	if err != nil || lease == nil {
		return presentation, err
	}
	// Admission already owns the shared remux. Readiness must not keep lifecycle
	// teardown from cancelling that remux, or outlive an abandoned HTTP request.
	manifest, release, err := lease.readManifest(ctx)
	if err != nil {
		return Presentation{}, err
	}
	return Presentation{Manifest: manifest, Release: release}, nil
}

func (o *Origin) acquireTune(ctx context.Context, request TuneRequest) (Presentation, *hlsPlaylistLease, error) {
	o.lifecycleMu.RLock()
	defer o.lifecycleMu.RUnlock()
	if err := o.checkAdmissionLocked(ctx, request.ChannelID); err != nil {
		return Presentation{}, nil, err
	}
	switch request.Delivery {
	case DeliveryMPEGTS:
		if o.sessions == nil {
			return Presentation{}, nil, ErrUnsupportedDelivery
		}
		stream, release, err := o.sessions.Attach(ctx, request.ChannelID, request.Plan)
		return Presentation{Stream: stream, Release: release}, nil, err
	case DeliveryHLS:
		if o.hls == nil {
			return Presentation{}, nil, ErrUnsupportedDelivery
		}
		lease, err := o.hls.acquirePlaylist(request.ChannelID, request.Plan, request.Speculative, ViewerFrom(ctx))
		return Presentation{}, &lease, err
	default:
		return Presentation{}, nil, ErrUnsupportedDelivery
	}
}

// OpenAsset opens a follow-up HLS resource without exposing the live remux layout to callers.
func (o *Origin) OpenAsset(ctx context.Context, channelID string, plan EncodePlan, rel string, speculative bool) (Asset, bool, error) {
	asset, lease, ok, err := o.acquireAsset(ctx, channelID, plan, rel, speculative)
	if err != nil || !ok || lease == nil {
		return asset, ok, err
	}
	// Like master readiness, variant readiness must permit lifecycle teardown to cancel the
	// packager it is waiting for. Only admission and retaining the lease require the lifecycle lock.
	body, release, err := lease.readManifest(ctx)
	if err != nil {
		return Asset{}, false, err
	}
	defer release()
	return Asset{Content: nopSeekCloser{bytes.NewReader(body)}, Modified: time.Now(), Playlist: true}, true, nil
}

func (o *Origin) acquireAsset(ctx context.Context, channelID string, plan EncodePlan, rel string, speculative bool) (Asset, *hlsPlaylistLease, bool, error) {
	o.lifecycleMu.RLock()
	defer o.lifecycleMu.RUnlock()
	if err := o.checkAdmissionLocked(ctx, channelID); err != nil {
		return Asset{}, nil, false, err
	}
	if o.hls == nil {
		return Asset{}, nil, false, nil
	}
	if mp, ok := o.hls.(mediaPlaylister); ok && strings.HasSuffix(rel, ".m3u8") {
		lease, ok, err := mp.acquireMediaPlaylist(ctx, channelID, plan, rel, speculative)
		return Asset{}, &lease, ok, err
	}
	path, ok := o.hls.AssetPath(channelID, plan, rel)
	if !ok {
		return Asset{}, nil, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return Asset{}, nil, false, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return Asset{}, nil, false, err
	}
	return Asset{Content: f, Modified: info.ModTime()}, nil, true, nil
}

// StopChannel retires every live delivery path for one channel. HLS is stopped first so it
// releases its session references and removes segment lookup state; the session manager then
// disconnects MPEG-TS viewers and kills any remaining encoder plans.
func (o *Origin) StopChannel(channelID string) {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	if o.hls != nil {
		o.hls.StopChannel(channelID)
	}
	if o.sessions != nil {
		o.sessions.StopChannel(channelID)
	}
}

// StopAll retires every live delivery without destroying the HLS scratch root, so the Origin can
// admit sessions again after a Postgres listener re-subscribes and completes durable catch-up.
func (o *Origin) StopAll() {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	o.stopAllLocked()
}

// Quiesce permanently closes this generation's admission and retires every live delivery. Unlike
// StopAll, which is reusable after a PostgreSQL listener recovers, Quiesce is the terminal
// application-generation boundary and is safe to call repeatedly.
func (o *Origin) Quiesce() {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	if o.closed {
		return
	}
	o.closed = true
	o.stopAllLocked()
}

func (o *Origin) stopAllLocked() {
	if o.hls != nil {
		o.hls.StopAll()
	}
	if o.sessions != nil {
		o.sessions.Stop()
	}
}

// nopSeekCloser is an in-memory Asset body.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }
