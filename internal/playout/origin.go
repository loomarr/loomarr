package playout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	// ErrPreparedUnavailable is a clean prepared-only miss. Callers use it to warm only media that
	// already exists without falling through to the live remux/encoder path.
	ErrPreparedUnavailable = errors.New("playout: prepared presentation unavailable")
)

// TuneRequest is everything a transport adapter must prove to tune a Channel. It deliberately
// carries no encoder, cache, or scratch-directory choice; those are Origin implementation details.
type TuneRequest struct {
	ChannelID string
	Plan      EncodePlan
	Delivery  Delivery
	// PreparedOnly forbids live fallback. It is a read-only probe for media already published by
	// the readiness control plane and must never create an encoder/remux session on a miss.
	PreparedOnly bool
}

// Presentation is one tuned Channel. Exactly one of Stream or Manifest is populated according
// to the requested Delivery. Release must be called when the caller is finished with this snapshot.
type Presentation struct {
	Stream   <-chan []byte
	Manifest []byte
	Release  func()
}

type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// Asset is an opened follow-up resource. Callers know its bytes and modification time, never its
// live or prepared filesystem layout.
type Asset struct {
	Content   readSeekCloser
	Modified  time.Time
	Immutable bool
}

// Admission is a tracked raw-transport operation. Context is cancelled when the request ends or
// lifecycle teardown retires its channel; Release must be called after the operation is finished.
// The unexported release function keeps registration ownership inside Origin while allowing HTTP
// adapters and shared test doubles to carry the lease without learning its bookkeeping.
type Admission struct {
	Context context.Context
	release func()
}

// Release retires the admission lease. It is safe to call on the zero value and more than once.
func (a Admission) Release() {
	if a.release != nil {
		a.release()
	}
}

type sessionAttacher interface {
	Attach(context.Context, string, EncodePlan) (<-chan []byte, func(), error)
	StopChannel(channelID string)
	Stop()
}

type hlsOrigin interface {
	Playlist(string, EncodePlan) (string, func(), error)
	AssetPath(string, EncodePlan, string) (string, bool)
	StopChannel(channelID string)
	StopAll()
}

type preparedDelivery interface {
	Tune(context.Context, TuneRequest) (Presentation, bool, error)
	OpenAsset(string, EncodePlan, string) (Asset, bool, error)
}

// Origin is the one playout seam used by transport adapters. The current live implementations are
// hidden behind it; prepared delivery can replace their selection without changing callers.
type Origin struct {
	prepared preparedDelivery
	sessions sessionAttacher
	hls      hlsOrigin
	observer OriginObserver

	// lifecycleMu orders admission plus attachment against fail-closed StopAll. The atomic
	// availability callback alone is insufficient: a tune could observe true, then attach after
	// StopAll had already snapshotted the managers and escape teardown.
	lifecycleMu sync.RWMutex
	closed      bool
	available   func() bool
	eligible    func(context.Context, string) (bool, error)

	admissionsMu sync.Mutex
	admissions   map[*admissionLease]string
}

type admissionLease struct {
	origin *Origin
	once   sync.Once
	cancel context.CancelFunc
}

func (l *admissionLease) release() {
	l.once.Do(func() {
		l.cancel()
		l.origin.admissionsMu.Lock()
		delete(l.origin.admissions, l)
		l.origin.admissionsMu.Unlock()
	})
}

// OriginDependencies are the implementations hidden behind the one production playout seam.
// Prepared is consulted first; the bounded live implementations remain internal fallbacks.
type OriginDependencies struct {
	Prepared     *PreparedOrigin
	LiveSessions *Manager
	LiveHLS      *HLSManager
	// Available is a fail-closed admission gate. Nil means always available (the SQLite
	// single-replica path); Postgres supplies a gate tied to its durable invalidation listener.
	Available func() bool
	// Eligible revalidates one channel at the playout seam. Postgres supplies a durable read so
	// an HTTP request admitted just before a remote commit cannot attach after that commit's stop.
	// Nil preserves the SQLite single-replica path's existing local lifecycle behavior.
	Eligible func(context.Context, string) (bool, error)
	Observer OriginObserver
}

// OriginObserver receives bounded fallback transitions without Channel identity.
type OriginObserver interface {
	PlayoutFallback(reason string)
}

// NewOrigin assembles prepared delivery and the current bounded live fallback behind one seam.
func NewOrigin(deps OriginDependencies) *Origin {
	// A nil concrete pointer stored directly in an interface is non-nil. Normalize every optional
	// implementation here so degraded construction cannot call through a typed-nil dependency.
	var prepared preparedDelivery
	if deps.Prepared != nil {
		prepared = deps.Prepared
	}
	var sessions sessionAttacher
	if deps.LiveSessions != nil {
		sessions = deps.LiveSessions
	}
	var hls hlsOrigin
	if deps.LiveHLS != nil {
		hls = deps.LiveHLS
	}
	o := newOrigin(prepared, sessions, hls)
	o.available = deps.Available
	o.eligible = deps.Eligible
	o.observer = deps.Observer
	return o
}

func newOrigin(prepared preparedDelivery, sessions sessionAttacher, hls hlsOrigin) *Origin {
	return &Origin{prepared: prepared, sessions: sessions, hls: hls}
}

// AcquireAdmission applies the same fail-closed lifecycle decision used by Tune and OpenAsset and
// registers a cancellable raw-transport operation before teardown can pass it. Internal program
// hops hold the returned lease through resolver and encoder work; StopChannel/StopAll cancel it.
func (o *Origin) AcquireAdmission(ctx context.Context, channelID string) (Admission, error) {
	if o == nil {
		return Admission{}, ErrUnavailable
	}
	o.lifecycleMu.RLock()
	defer o.lifecycleMu.RUnlock()
	if err := o.checkAdmissionLocked(ctx, channelID); err != nil {
		return Admission{}, err
	}
	leaseCtx, cancel := context.WithCancel(ctx)
	lease := &admissionLease{origin: o, cancel: cancel}
	o.admissionsMu.Lock()
	if o.admissions == nil {
		o.admissions = make(map[*admissionLease]string)
	}
	o.admissions[lease] = channelID
	o.admissionsMu.Unlock()
	return Admission{Context: leaseCtx, release: lease.release}, nil
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
	o.lifecycleMu.RLock()
	defer o.lifecycleMu.RUnlock()
	if err := o.checkAdmissionLocked(ctx, request.ChannelID); err != nil {
		return Presentation{}, err
	}
	var preparedErr error
	if request.Delivery == DeliveryHLS && o.prepared != nil {
		presentation, hit, err := o.prepared.Tune(ctx, request)
		if err == nil && hit {
			return presentation, nil
		}
		preparedErr = err
	}
	if request.Delivery == DeliveryHLS && request.PreparedOnly {
		if preparedErr != nil {
			return Presentation{}, preparedErr
		}
		return Presentation{}, ErrPreparedUnavailable
	}
	switch request.Delivery {
	case DeliveryMPEGTS:
		if o.sessions == nil {
			return Presentation{}, ErrUnsupportedDelivery
		}
		stream, release, err := o.sessions.Attach(ctx, request.ChannelID, request.Plan)
		return Presentation{Stream: stream, Release: release}, err
	case DeliveryHLS:
		if o.hls == nil {
			if preparedErr != nil {
				return Presentation{}, preparedErr
			}
			return Presentation{}, ErrUnsupportedDelivery
		}
		path, release, err := o.hls.Playlist(request.ChannelID, request.Plan)
		if err != nil {
			return Presentation{}, err
		}
		manifest, err := os.ReadFile(path)
		if err != nil {
			release()
			return Presentation{}, fmt.Errorf("playout: read manifest: %w", err)
		}
		if o.prepared != nil && o.observer != nil {
			o.observer.PlayoutFallback("prepared_to_live")
		}
		return Presentation{Manifest: manifest, Release: release}, nil
	default:
		return Presentation{}, ErrUnsupportedDelivery
	}
}

// OpenAsset opens a follow-up HLS resource without exposing the live remux layout to callers.
func (o *Origin) OpenAsset(ctx context.Context, channelID string, plan EncodePlan, rel string) (Asset, bool, error) {
	o.lifecycleMu.RLock()
	defer o.lifecycleMu.RUnlock()
	if err := o.checkAdmissionLocked(ctx, channelID); err != nil {
		return Asset{}, false, err
	}
	if o.prepared != nil {
		asset, ok, err := o.prepared.OpenAsset(channelID, plan, rel)
		if err != nil || ok {
			return asset, ok, err
		}
	}
	if o.hls == nil {
		return Asset{}, false, nil
	}
	path, ok := o.hls.AssetPath(channelID, plan, rel)
	if !ok {
		return Asset{}, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return Asset{}, false, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return Asset{}, false, err
	}
	return Asset{Content: f, Modified: info.ModTime()}, true, nil
}

// StopChannel retires every live delivery path for one channel. HLS is stopped first so it
// releases its session references and removes segment lookup state; the session manager then
// disconnects MPEG-TS viewers and kills any remaining encoder plans. Prepared assets are
// immutable cache entries, not live sessions, and remain available for later resume.
func (o *Origin) StopChannel(channelID string) {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	o.cancelAdmissions(channelID)
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
	o.cancelAdmissions("")
	if o.hls != nil {
		o.hls.StopAll()
	}
	if o.sessions != nil {
		o.sessions.Stop()
	}
}

// cancelAdmissions cancels every raw operation for channelID; an empty id means all channels.
// lifecycleMu is held by the caller, so an admission that passed its durable check cannot register
// after this snapshot and escape teardown.
func (o *Origin) cancelAdmissions(channelID string) {
	o.admissionsMu.Lock()
	defer o.admissionsMu.Unlock()
	for lease, admittedChannel := range o.admissions {
		if channelID == "" || admittedChannel == channelID {
			lease.cancel()
			delete(o.admissions, lease)
		}
	}
}
