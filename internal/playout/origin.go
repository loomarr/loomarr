package playout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

type sessionAttacher interface {
	Attach(context.Context, string, EncodePlan) (<-chan []byte, func(), error)
	StopChannel(channelID string)
}

type hlsOrigin interface {
	Playlist(string, EncodePlan) (string, func(), error)
	AssetPath(string, EncodePlan, string) (string, bool)
	StopChannel(channelID string)
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
}

// OriginDependencies are the implementations hidden behind the one production playout seam.
// Prepared is consulted first; the bounded live implementations remain internal fallbacks.
type OriginDependencies struct {
	Prepared     *PreparedOrigin
	LiveSessions *Manager
	LiveHLS      *HLSManager
}

// NewOrigin assembles prepared delivery and the current bounded live fallback behind one seam.
func NewOrigin(deps OriginDependencies) *Origin {
	return newOrigin(deps.Prepared, deps.LiveSessions, deps.LiveHLS)
}

func newOrigin(prepared preparedDelivery, sessions sessionAttacher, hls hlsOrigin) *Origin {
	return &Origin{prepared: prepared, sessions: sessions, hls: hls}
}

// Tune returns the presentation for the requested delivery without exposing which implementation
// produced it.
func (o *Origin) Tune(ctx context.Context, request TuneRequest) (Presentation, error) {
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
		return Presentation{Manifest: manifest, Release: release}, nil
	default:
		return Presentation{}, ErrUnsupportedDelivery
	}
}

// OpenAsset opens a follow-up HLS resource without exposing the live remux layout to callers.
func (o *Origin) OpenAsset(channelID string, plan EncodePlan, rel string) (Asset, bool, error) {
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
	if o.hls != nil {
		o.hls.StopChannel(channelID)
	}
	if o.sessions != nil {
		o.sessions.StopChannel(channelID)
	}
}
