package diagnostics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// NewSlogHandler fans accepted slog records to independent stdout and durable sinks. The caller
// should place its dynamic secret-redaction handler outside this one so both projections see the
// same scrubbed record. Persistence is non-blocking and cannot change the stdout handler's result.
func NewSlogHandler(stdout slog.Handler, recorder *Recorder) slog.Handler {
	return &slogHandler{stdout: stdout, recorder: recorder}
}

type slogHandler struct {
	stdout   slog.Handler
	recorder *Recorder
	attrs    map[string]any
	groups   []string
}

func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return (h.stdout != nil && h.stdout.Enabled(ctx, level)) || h.recorder.Accepts(slogLevel(level))
}

func (h *slogHandler) Handle(ctx context.Context, record slog.Record) error {
	var stdoutErr error
	if h.stdout != nil && h.stdout.Enabled(ctx, record.Level) {
		stdoutErr = h.stdout.Handle(ctx, record)
	}
	level := slogLevel(record.Level)
	if !h.recorder.Accepts(level) {
		return stdoutErr
	}

	values := make(map[string]any, len(h.attrs)+record.NumAttrs())
	for key, value := range h.attrs {
		values[key] = value
	}
	record.Attrs(func(attr slog.Attr) bool {
		collectSlogAttr(values, h.groups, attr)
		return true
	})

	event := Event{
		OccurredAt: record.Time,
		Level:      level,
		Source:     SourceServer,
		Name:       "server.log",
		Message:    record.Message,
		Attributes: values,
	}
	promoteSlogFields(&event)
	h.recorder.Record(ctx, event)
	return stdoutErr
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = make(map[string]any, len(h.attrs)+len(attrs))
	for key, value := range h.attrs {
		clone.attrs[key] = value
	}
	for _, attr := range attrs {
		collectSlogAttr(clone.attrs, h.groups, attr)
	}
	if h.stdout != nil {
		clone.stdout = h.stdout.WithAttrs(attrs)
	}
	return &clone
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	if h.stdout != nil {
		clone.stdout = h.stdout.WithGroup(name)
	}
	return &clone
}

func slogLevel(level slog.Level) Level {
	switch {
	case level < slog.LevelInfo:
		return LevelDebug
	case level < slog.LevelWarn:
		return LevelInfo
	case level < slog.LevelError:
		return LevelWarn
	default:
		return LevelError
	}
}

func collectSlogAttr(out map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		nested := groups
		if attr.Key != "" {
			nested = append(append([]string(nil), groups...), attr.Key)
		}
		for _, child := range attr.Value.Group() {
			collectSlogAttr(out, nested, child)
		}
		return
	}
	keyParts := append(append([]string(nil), groups...), attr.Key)
	key := strings.Join(keyParts, ".")
	if key == "" {
		return
	}
	out[key] = slogAttributeValue(attr.Value)
}

func slogAttributeValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		if stringer, ok := value.Any().(fmt.Stringer); ok {
			return stringer.String()
		}
		return value.Any()
	default:
		return value.String()
	}
}

func promoteSlogFields(event *Event) {
	promote := func(target *string, keys ...string) {
		for _, key := range keys {
			value, ok := event.Attributes[key]
			if !ok {
				continue
			}
			text, ok := value.(string)
			if !ok {
				continue
			}
			*target = text
			delete(event.Attributes, key)
			return
		}
	}
	promote(&event.Name, "event")
	promote(&event.Subsystem, "subsystem")
	promote(&event.RequestID, "request_id", "requestId")
	promote(&event.PlaybackSessionID, "playback_session_id", "playbackSessionId")
	promote(&event.ChannelID, "channel_id", "channelId")
	promote(&event.ScheduleBlockID, "schedule_block_id", "scheduleBlockId")
	promote(&event.JobID, "job_id", "jobId")
	promote(&event.ProcessRunID, "process_run_id", "processRunId")
	promote(&event.ActorID, "actor_id", "actorId")
	promote(&event.InstanceID, "instance_id", "instanceId")
	var source string
	promote(&source, "source")
	switch Source(source) {
	case SourceServer, SourceWeb, SourceAndroidMobile, SourceAndroidTV:
		event.Source = Source(source)
	case "":
	default:
		// An invalid override is still useful evidence; retain it as an attribute while the
		// observation honestly remains server-sourced.
		event.Attributes["source"] = source
	}
}
