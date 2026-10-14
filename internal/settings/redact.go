package settings

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// Redactor holds the current set of secret values and replaces any occurrence in
// a log line before write (config-design §4: redaction is systemic, not
// per-callsite). It is fed by both the generated Secrets and the app-managed
// secret settings (Emby token, Seerr key, …) so no secret — minted or entered —
// leaks into logs, error bodies, or /v1/setup/status.
type Redactor struct {
	mu      sync.RWMutex
	sources map[string][]string
	values  []string // flattened, non-empty secret values to scrub
}

// NewRedactor builds an empty redactor. Sources register their secrets via Set.
func NewRedactor() *Redactor { return &Redactor{} }

// Set replaces the tracked secret values (called on boot and after any secret
// change, so a freshly-saved token is scrubbed immediately). Only non-empty,
// sufficiently-long values are tracked — a 1-char "secret" would scrub every
// line to noise; the floor avoids pathological redaction.
func (r *Redactor) Set(values []string) {
	r.SetSource("settings", values)
}

// SetSource replaces one source's tracked values without disturbing secrets registered by other
// subsystems. This keeps a settings refresh from dropping provider credentials, and lets a
// provider refresh replace only its own snapshot. Longer values are matched first so one secret
// that prefixes another cannot leave an unredacted suffix behind.
func (r *Redactor) SetSource(source string, values []string) {
	filtered := make([]string, 0, len(values))
	for _, v := range values {
		if len(v) >= 8 { // secrets are 256-bit/entered keys — never this short by accident
			filtered = append(filtered, v)
		}
	}
	r.mu.Lock()
	if r.sources == nil {
		r.sources = make(map[string][]string)
	}
	if len(filtered) == 0 {
		delete(r.sources, source)
	} else {
		r.sources[source] = filtered
	}
	unique := make(map[string]struct{})
	flattened := make([]string, 0)
	for _, sourceValues := range r.sources {
		for _, value := range sourceValues {
			if _, exists := unique[value]; exists {
				continue
			}
			unique[value] = struct{}{}
			flattened = append(flattened, value)
		}
	}
	sort.Slice(flattened, func(i, j int) bool { return len(flattened[i]) > len(flattened[j]) })
	r.values = flattened
	r.mu.Unlock()
}

// redact scrubs every tracked secret value from s, replacing it with "‹redacted›".
func (r *Redactor) redact(s string) string {
	r.mu.RLock()
	vals := r.values
	r.mu.RUnlock()
	for _, v := range vals {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, "‹redacted›")
		}
	}
	return s
}

// Handler wraps a slog.Handler so every attribute value (and message) is scrubbed
// of tracked secrets before the inner handler writes it (config-design §4).
func (r *Redactor) Handler(inner slog.Handler) slog.Handler {
	return &redactHandler{inner: inner, r: r}
}

type redactHandler struct {
	inner slog.Handler
	r     *Redactor
	ops   []redactHandlerOp
}

type redactHandlerOp struct {
	group string
	attrs []slog.Attr
}

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	inner := h.inner
	for _, op := range h.ops {
		if op.group != "" {
			inner = inner.WithGroup(op.group)
			continue
		}
		attrs := make([]slog.Attr, len(op.attrs))
		for i, attr := range op.attrs {
			attrs[i] = h.redactAttr(attr)
		}
		inner = inner.WithAttrs(attrs)
	}
	// Rebuild the record with a scrubbed message and scrubbed attribute values.
	out := slog.NewRecord(rec.Time, rec.Level, h.r.redact(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.redactAttr(a))
		return true
	})
	return inner.Handle(ctx, out)
}

// redactAttr scrubs an attribute's string form. Non-string values are rendered
// via their String() and scrubbed too, so a secret smuggled through a struct's
// stringer doesn't slip past.
func (h *redactHandler) redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		children := v.Group()
		redacted := make([]slog.Attr, len(children))
		for i, child := range children {
			redacted[i] = h.redactAttr(child)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redacted...)}
	}
	if v.Kind() == slog.KindString {
		return slog.String(a.Key, h.r.redact(v.String()))
	}
	// For any other kind, scrub its string rendering only if it actually contains
	// a secret (avoid turning ints/bools into strings needlessly).
	str := v.String()
	if scrubbed := h.r.redact(str); scrubbed != str {
		return slog.String(a.Key, scrubbed)
	}
	return slog.Attr{Key: a.Key, Value: v}
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.ops = append(append([]redactHandlerOp(nil), h.ops...), redactHandlerOp{
		attrs: append([]slog.Attr(nil), attrs...),
	})
	return &clone
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.ops = append(append([]redactHandlerOp(nil), h.ops...), redactHandlerOp{group: name})
	return &clone
}
