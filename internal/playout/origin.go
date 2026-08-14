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

var ErrUnsupportedDelivery = errors.New("playout: unsupported delivery")

// TuneRequest is everything a transport adapter must prove to tune a Channel. It deliberately
// carries no encoder, cache, or scratch-directory choice; those are Origin implementation details.
type TuneRequest struct {
	ChannelID string
	Plan      EncodePlan
	Delivery  Delivery
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
	Content  readSeekCloser
	Modified time.Time
}

type sessionAttacher interface {
	Attach(context.Context, string, EncodePlan) (<-chan []byte, func(), error)
}

type hlsOrigin interface {
	Playlist(string, EncodePlan) (string, func(), error)
	AssetPath(string, EncodePlan, string) (string, bool)
}

// Origin is the one playout seam used by transport adapters. The current live implementations are
// hidden behind it; prepared delivery can replace their selection without changing callers.
type Origin struct {
	sessions sessionAttacher
	hls      hlsOrigin
}

// NewOrigin places the existing live session and HLS implementations behind the production seam.
func NewOrigin(sessions *Manager, hls *HLSManager) *Origin { return newOrigin(sessions, hls) }

func newOrigin(sessions sessionAttacher, hls hlsOrigin) *Origin {
	return &Origin{sessions: sessions, hls: hls}
}

// Tune returns the presentation for the requested delivery without exposing which implementation
// produced it.
func (o *Origin) Tune(ctx context.Context, request TuneRequest) (Presentation, error) {
	switch request.Delivery {
	case DeliveryMPEGTS:
		if o.sessions == nil {
			return Presentation{}, ErrUnsupportedDelivery
		}
		stream, release, err := o.sessions.Attach(ctx, request.ChannelID, request.Plan)
		return Presentation{Stream: stream, Release: release}, err
	case DeliveryHLS:
		if o.hls == nil {
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
