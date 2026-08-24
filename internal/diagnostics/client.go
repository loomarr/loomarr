package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/time/rate"
)

const (
	maxClientBatch       = 20
	maxClientTokenBytes  = 128
	maxClientLabelBytes  = 64
	clientEventsPerSec   = 2
	clientEventBurst     = 30
	maxClientClockSkew   = 365 * 24 * time.Hour
	maxClientPlaybackAge = 7 * 24 * time.Hour
	maxClientDrift       = 24 * time.Hour
	maxClientBuffer      = 30 * time.Minute
	ClientErrorBoundary  = "client.error_boundary"
	ClientUnhandledError = "client.unhandled_error"
	ClientAPIFailed      = "client.api_failed"
	PlayerAttached       = "player.attached"
	PlayerDetached       = "player.detached"
	PlayerReady          = "player.ready"
	PlayerSourceReplaced = "player.source_replaced"
	PlayerBufferingStart = "player.buffering_started"
	PlayerBufferingEnd   = "player.buffering_ended"
	PlayerSeeking        = "player.seeking"
	PlayerSeeked         = "player.seeked"
	PlayerMediaError     = "player.media_error"
	PlayerBlockChanged   = "player.schedule_block_changed"
	PlayerPlayheadDrift  = "player.playhead_drift"
)

var (
	ErrInvalidClientBatch = errors.New("invalid client diagnostic batch")
	ErrClientRateLimited  = errors.New("client diagnostics rate limited")
	ErrClientUnavailable  = errors.New("client diagnostics unavailable")
)

// ClientBatch is the complete wire-safe client submission. Actor, receipt time, level, subsystem,
// message and arbitrary attributes are deliberately absent: the server owns all of them.
type ClientBatch struct {
	Source        Source              `json:"source" enum:"web,android_tv"`
	ClientVersion string              `json:"clientVersion" maxLength:"64"`
	Platform      string              `json:"platform" enum:"chromium,firefox,webkit,unknown_web,android_tv,shield_tv,unknown_android_tv"`
	Events        []ClientObservation `json:"events" minItems:"1" maxItems:"20"`
}

// ClientObservation is a closed union represented as a flat wire shape. validateClientObservation
// enforces the event-specific subset, so a field valid for one event is rejected on another.
type ClientObservation struct {
	Event                   string `json:"event" enum:"client.error_boundary,client.unhandled_error,client.api_failed,player.attached,player.detached,player.ready,player.source_replaced,player.buffering_started,player.buffering_ended,player.seeking,player.seeked,player.media_error,player.schedule_block_changed,player.playhead_drift"`
	OccurredAt              int64  `json:"occurredAt" minimum:"1"`
	RequestID               string `json:"requestId,omitempty" maxLength:"128"`
	PlaybackSessionID       string `json:"playbackSessionId,omitempty" maxLength:"128"`
	ChannelID               string `json:"channelId,omitempty" maxLength:"128"`
	ScheduleBlockID         string `json:"scheduleBlockId,omitempty" maxLength:"128"`
	PreviousScheduleBlockID string `json:"previousScheduleBlockId,omitempty" maxLength:"128"`
	PreviousChannelID       string `json:"previousChannelId,omitempty" maxLength:"128"`
	Transport               string `json:"transport,omitempty" enum:"native_hls,hls_js,media3"`
	Reason                  string `json:"reason,omitempty" enum:"mount,unmount,channel_change,retry,expired,unsupported,network,media,unknown"`
	Surface                 string `json:"surface,omitempty" enum:"root,router,watch,guide,settings,unknown"`
	ErrorClass              string `json:"errorClass,omitempty" enum:"error,type_error,range_error,promise_rejection,react_error,playback_exception,io_exception,unknown"`
	ErrorCode               string `json:"errorCode,omitempty" maxLength:"64"`
	BlockKind               string `json:"blockKind,omitempty" enum:"program,filler,flex"`
	HTTPStatus              *int   `json:"httpStatus,omitempty" minimum:"100" maximum:"599"`
	Fatal                   *bool  `json:"fatal,omitempty"`
	ViewerTimeMs            *int64 `json:"viewerTimeMs,omitempty" minimum:"0"`
	ServerTimeMs            *int64 `json:"serverTimeMs,omitempty" minimum:"0"`
	DriftMs                 *int64 `json:"driftMs,omitempty"`
	BufferedMs              *int64 `json:"bufferedMs,omitempty" minimum:"0"`
}

// UnmarshalJSON fails closed on unknown fields. Huma validates declared fields, while this method
// prevents an authenticated client from smuggling arbitrary console/DOM/header data beside them.
func (b *ClientBatch) UnmarshalJSON(data []byte) error {
	type wire ClientBatch
	return decodeClosedJSON(data, (*wire)(b))
}

func (e *ClientObservation) UnmarshalJSON(data []byte) error {
	type wire ClientObservation
	return decodeClosedJSON(data, (*wire)(e))
}

func decodeClosedJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// ClientIngestor owns the entire untrusted-client admission contract behind one operation.
type ClientIngestor struct {
	recorder *Recorder
	now      func() time.Time

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewClientIngestor(recorder *Recorder, now func() time.Time) *ClientIngestor {
	if now == nil {
		now = time.Now
	}
	return &ClientIngestor{recorder: recorder, now: now, limiters: make(map[string]*rate.Limiter)}
}

// Ingest validates the whole batch, charges the server-derived actor, then performs only
// non-blocking Recorder admissions. A validation or rate error records nothing.
func (i *ClientIngestor) Ingest(ctx context.Context, actorID string, batch ClientBatch) (int, error) {
	if i == nil || i.recorder == nil || !i.recorder.Accepts(LevelInfo) {
		return 0, ErrClientUnavailable
	}
	now := i.now()
	events, err := validateClientBatch(actorID, batch, now)
	if err != nil {
		return 0, err
	}
	if !i.allow(actorID, len(events), now) {
		return 0, ErrClientRateLimited
	}
	for _, event := range events {
		i.recorder.Record(ctx, event)
	}
	return len(events), nil
}

func (i *ClientIngestor) allow(actorID string, count int, now time.Time) bool {
	i.mu.Lock()
	limiter := i.limiters[actorID]
	if limiter == nil {
		limiter = rate.NewLimiter(clientEventsPerSec, clientEventBurst)
		i.limiters[actorID] = limiter
	}
	allowed := limiter.AllowN(now, count)
	i.mu.Unlock()
	return allowed
}

func validateClientBatch(actorID string, batch ClientBatch, now time.Time) ([]Event, error) {
	if !validIdentifier(actorID) {
		return nil, invalidClientBatch("server-derived actor is unavailable")
	}
	if batch.Source != SourceWeb && batch.Source != SourceAndroidTV {
		return nil, invalidClientBatch("source must be web or android_tv")
	}
	if !validClientVersion(batch.ClientVersion) {
		return nil, invalidClientBatch("clientVersion is invalid")
	}
	if !validClientPlatform(batch.Source, batch.Platform) {
		return nil, invalidClientBatch("platform does not match source")
	}
	if len(batch.Events) < 1 || len(batch.Events) > maxClientBatch {
		return nil, invalidClientBatch("events must contain between 1 and 20 observations")
	}
	out := make([]Event, 0, len(batch.Events))
	for index, observation := range batch.Events {
		event, err := validateClientObservation(batch, actorID, observation, now)
		if err != nil {
			return nil, invalidClientBatch(fmt.Sprintf("events[%d]: %v", index, err))
		}
		out = append(out, event)
	}
	return out, nil
}

func validateClientObservation(batch ClientBatch, actorID string, in ClientObservation, now time.Time) (Event, error) {
	if in.OccurredAt <= 0 {
		return Event{}, errors.New("occurredAt is required")
	}
	occurred := time.UnixMilli(in.OccurredAt)
	if skew := occurred.Sub(now); skew > maxClientClockSkew || skew < -maxClientClockSkew {
		return Event{}, errors.New("occurredAt is outside the accepted clock-skew window")
	}
	for name, value := range map[string]string{
		"requestId": in.RequestID, "playbackSessionId": in.PlaybackSessionID,
		"channelId": in.ChannelID, "scheduleBlockId": in.ScheduleBlockID,
		"previousScheduleBlockId": in.PreviousScheduleBlockID, "previousChannelId": in.PreviousChannelID,
	} {
		if value != "" && !validIdentifier(value) {
			return Event{}, fmt.Errorf("%s is invalid", name)
		}
	}
	if strings.HasPrefix(in.Event, "player.") && (in.PlaybackSessionID == "" || in.ChannelID == "") {
		return Event{}, errors.New("player events require playbackSessionId and channelId")
	}
	if in.ErrorCode != "" && !validToken(in.ErrorCode, maxClientLabelBytes) {
		return Event{}, errors.New("errorCode is invalid")
	}
	if in.ViewerTimeMs != nil && (*in.ViewerTimeMs < 0 || *in.ViewerTimeMs > maxClientPlaybackAge.Milliseconds()+now.UnixMilli()) {
		return Event{}, errors.New("viewerTimeMs is out of range")
	}
	if in.ServerTimeMs != nil && (*in.ServerTimeMs < 0 || math.Abs(float64(*in.ServerTimeMs-now.UnixMilli())) > float64(maxClientClockSkew.Milliseconds())) {
		return Event{}, errors.New("serverTimeMs is out of range")
	}
	if in.DriftMs != nil && (*in.DriftMs < -maxClientDrift.Milliseconds() || *in.DriftMs > maxClientDrift.Milliseconds()) {
		return Event{}, errors.New("driftMs is out of range")
	}
	if in.BufferedMs != nil && (*in.BufferedMs < 0 || *in.BufferedMs > maxClientBuffer.Milliseconds()) {
		return Event{}, errors.New("bufferedMs is out of range")
	}

	level, subsystem, attrs, err := clientEventProjection(in)
	if err != nil {
		return Event{}, err
	}
	attrs["client_version"] = batch.ClientVersion
	attrs["platform"] = batch.Platform
	return Event{
		OccurredAt: occurred, Level: level, Source: batch.Source, Subsystem: subsystem, Name: in.Event,
		RequestID: in.RequestID, PlaybackSessionID: in.PlaybackSessionID, ChannelID: in.ChannelID,
		ScheduleBlockID: in.ScheduleBlockID, ActorID: actorID, Attributes: attrs,
	}, nil
}

func clientEventProjection(in ClientObservation) (Level, string, map[string]any, error) {
	attrs := map[string]any{}
	add := func(name string, value any) { attrs[name] = value }
	no := func(names ...string) error {
		for _, name := range names {
			if clientFieldPresent(in, name) {
				return fmt.Errorf("%s is not allowed for %s", name, in.Event)
			}
		}
		return nil
	}
	allSpecific := []string{"previousScheduleBlockId", "previousChannelId", "transport", "reason", "surface", "errorClass", "errorCode", "blockKind", "httpStatus", "fatal", "viewerTimeMs", "serverTimeMs", "driftMs", "bufferedMs"}
	allowOnly := func(allowed ...string) error {
		set := make(map[string]bool, len(allowed))
		for _, name := range allowed {
			set[name] = true
		}
		for _, name := range allSpecific {
			if !set[name] && clientFieldPresent(in, name) {
				return fmt.Errorf("%s is not allowed for %s", name, in.Event)
			}
		}
		return nil
	}

	switch in.Event {
	case ClientErrorBoundary, ClientUnhandledError:
		if err := allowOnly("surface", "errorClass"); err != nil {
			return "", "", nil, err
		}
		if in.Surface == "" || in.ErrorClass == "" {
			return "", "", nil, errors.New("surface and errorClass are required")
		}
		add("surface", in.Surface)
		add("error_class", in.ErrorClass)
		return LevelError, "client", attrs, nil
	case ClientAPIFailed:
		if err := allowOnly("httpStatus"); err != nil {
			return "", "", nil, err
		}
		if in.RequestID == "" || in.HTTPStatus == nil || *in.HTTPStatus < 100 || *in.HTTPStatus > 599 {
			return "", "", nil, errors.New("requestId and a valid httpStatus are required")
		}
		add("http_status", *in.HTTPStatus)
		return LevelWarn, "api", attrs, nil
	case PlayerAttached, PlayerDetached, PlayerReady:
		if err := allowOnly("transport", "reason"); err != nil {
			return "", "", nil, err
		}
		if in.Transport == "" {
			return "", "", nil, errors.New("transport is required")
		}
		add("transport", in.Transport)
		if in.Reason != "" {
			add("reason", in.Reason)
		}
		return LevelInfo, "playback", attrs, nil
	case PlayerSourceReplaced:
		if err := allowOnly("transport", "reason", "previousChannelId"); err != nil {
			return "", "", nil, err
		}
		if in.Transport == "" || in.PreviousChannelID == "" || in.Reason != "channel_change" {
			return "", "", nil, errors.New("transport, previousChannelId, and channel_change reason are required")
		}
		add("transport", in.Transport)
		add("reason", in.Reason)
		add("previous_channel_id", in.PreviousChannelID)
		return LevelInfo, "playback", attrs, nil
	case PlayerBufferingStart, PlayerBufferingEnd:
		if err := allowOnly("transport", "viewerTimeMs", "bufferedMs"); err != nil {
			return "", "", nil, err
		}
		if in.Transport == "" || in.ViewerTimeMs == nil {
			return "", "", nil, errors.New("transport and viewerTimeMs are required")
		}
		add("transport", in.Transport)
		add("viewer_time_ms", *in.ViewerTimeMs)
		if in.BufferedMs != nil {
			add("buffered_ms", *in.BufferedMs)
		}
		return LevelInfo, "playback", attrs, nil
	case PlayerSeeking, PlayerSeeked:
		if err := allowOnly("transport", "viewerTimeMs"); err != nil {
			return "", "", nil, err
		}
		if in.Transport == "" || in.ViewerTimeMs == nil {
			return "", "", nil, errors.New("transport and viewerTimeMs are required")
		}
		add("transport", in.Transport)
		add("viewer_time_ms", *in.ViewerTimeMs)
		return LevelInfo, "playback", attrs, nil
	case PlayerMediaError:
		if err := allowOnly("transport", "errorCode", "fatal"); err != nil {
			return "", "", nil, err
		}
		if in.Transport == "" || in.ErrorCode == "" || in.Fatal == nil {
			return "", "", nil, errors.New("transport, errorCode, and fatal are required")
		}
		add("transport", in.Transport)
		add("error_code", in.ErrorCode)
		add("fatal", *in.Fatal)
		if *in.Fatal {
			return LevelError, "playback", attrs, nil
		}
		return LevelWarn, "playback", attrs, nil
	case PlayerBlockChanged:
		if err := allowOnly("previousScheduleBlockId", "blockKind", "viewerTimeMs", "serverTimeMs"); err != nil {
			return "", "", nil, err
		}
		if in.ScheduleBlockID == "" || in.BlockKind == "" || in.ViewerTimeMs == nil || in.ServerTimeMs == nil {
			return "", "", nil, errors.New("scheduleBlockId, blockKind, viewerTimeMs, and serverTimeMs are required")
		}
		if in.PreviousScheduleBlockID != "" {
			add("previous_schedule_block_id", in.PreviousScheduleBlockID)
		}
		add("block_kind", in.BlockKind)
		add("viewer_time_ms", *in.ViewerTimeMs)
		add("server_time_ms", *in.ServerTimeMs)
		return LevelInfo, "playback", attrs, nil
	case PlayerPlayheadDrift:
		if err := allowOnly("viewerTimeMs", "serverTimeMs", "driftMs"); err != nil {
			return "", "", nil, err
		}
		if in.ViewerTimeMs == nil || in.ServerTimeMs == nil || in.DriftMs == nil {
			return "", "", nil, errors.New("viewerTimeMs, serverTimeMs, and driftMs are required")
		}
		if *in.DriftMs != *in.ViewerTimeMs-*in.ServerTimeMs {
			return "", "", nil, errors.New("driftMs must equal viewerTimeMs minus serverTimeMs")
		}
		add("viewer_time_ms", *in.ViewerTimeMs)
		add("server_time_ms", *in.ServerTimeMs)
		add("drift_ms", *in.DriftMs)
		return LevelWarn, "playback", attrs, nil
	default:
		if err := no(allSpecific...); err != nil {
			return "", "", nil, err
		}
		return "", "", nil, errors.New("event is not recognized")
	}
}

func clientFieldPresent(in ClientObservation, name string) bool {
	switch name {
	case "previousScheduleBlockId":
		return in.PreviousScheduleBlockID != ""
	case "previousChannelId":
		return in.PreviousChannelID != ""
	case "transport":
		return in.Transport != ""
	case "reason":
		return in.Reason != ""
	case "surface":
		return in.Surface != ""
	case "errorClass":
		return in.ErrorClass != ""
	case "errorCode":
		return in.ErrorCode != ""
	case "blockKind":
		return in.BlockKind != ""
	case "httpStatus":
		return in.HTTPStatus != nil
	case "fatal":
		return in.Fatal != nil
	case "viewerTimeMs":
		return in.ViewerTimeMs != nil
	case "serverTimeMs":
		return in.ServerTimeMs != nil
	case "driftMs":
		return in.DriftMs != nil
	case "bufferedMs":
		return in.BufferedMs != nil
	default:
		return false
	}
}

func validClientPlatform(source Source, platform string) bool {
	switch source {
	case SourceWeb:
		return platform == "chromium" || platform == "firefox" || platform == "webkit" || platform == "unknown_web"
	case SourceAndroidTV:
		return platform == "android_tv" || platform == "shield_tv" || platform == "unknown_android_tv"
	default:
		return false
	}
}

func validClientVersion(value string) bool {
	return validToken(value, maxClientLabelBytes)
}

func validIdentifier(value string) bool {
	return validToken(value, maxClientTokenBytes)
}

func validToken(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._:+-", r) {
			continue
		}
		return false
	}
	return true
}

func invalidClientBatch(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidClientBatch, detail)
}
