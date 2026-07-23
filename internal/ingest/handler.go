package ingest

import (
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// Clock supplies the current time; injected for deterministic tests.
type Clock func() time.Time

// Emitter receives the terminal domain events a webhook produces (§4 inv. 2),
// so the scheduler can backfill the affected channels and the UI can reflect the
// change (§7 SSE). It's a local port (structural typing) so the composition root
// can hand ingest and the reconciler the SAME fan-out adapter without ingest
// importing a sibling subsystem. nil is allowed — events are a latency
// optimization, never load-bearing (the channel sweep recovers regardless, §9).
type Emitter interface {
	Emit(ctx context.Context, ev provision.DomainEvent)
}

// Handler serves POST /hooks/arr (§6). It verifies the shared secret, parses the
// webhook, and drives the provisioning state machine (§4), confirming library
// presence before marking a title available (invariant 4).
type Handler struct {
	store          store.Store
	lib            library.Library
	secret         string
	downloadingTTL time.Duration
	emit           Emitter
	now            Clock
	log            *slog.Logger
}

// New builds the ingest handler. emit may be nil (events are non-load-bearing);
// when set, terminal transitions fan out to the scheduler + event bus.
func New(st store.Store, lib library.Library, secret string, downloadingTTL time.Duration, emit Emitter, now Clock, log *slog.Logger) *Handler {
	return &Handler{store: st, lib: lib, secret: secret, downloadingTTL: downloadingTTL, emit: emit, now: now, log: log}
}

// ServeHTTP handles POST /hooks/arr?token=<secret>. Always acks 200 for accepted
// payloads (Test, tracked, or untracked) so the *arr app doesn't retry-storm;
// only a bad secret (401) or malformed body (400) is non-200 (§6).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This endpoint is Sonarr/Radarr-facing (a machine, not a person), so the messages
	// stay terse — but capitalized + specific enough to read in an *arr connection test log.
	if !h.validSecret(r) {
		http.Error(w, "Invalid or missing webhook secret.", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Couldn't read the request body.", http.StatusBadRequest)
		return
	}
	p, err := parse(body)
	if err != nil {
		http.Error(w, "Couldn't parse the webhook payload.", http.StatusBadRequest)
		return
	}
	metrics.WebhookEvent(webhookKind(p)) // §17: events by type (bounded)

	switch {
	case p.isTest:
		// Record a per-app last-received timestamp (powers the §13 onboarding
		// handshake). Ack 200 — never resolve a title from a Test (§6).
		if p.app != "" {
			_ = h.store.SetSetting(r.Context(), "webhook_last_received:"+p.app, h.now().UTC().Format(time.RFC3339))
		}
		w.WriteHeader(http.StatusOK)
		return
	case !p.isTracked:
		// Unknown/untracked payload — ack and ignore (§6).
		w.WriteHeader(http.StatusOK)
		return
	}

	h.handleTracked(r.Context(), p)
	w.WriteHeader(http.StatusOK)
}

// webhookKind classifies a parsed webhook into a bounded metric label (§17), so
// an arbitrary eventType string can never inflate the /metrics cardinality.
func webhookKind(p parsed) string {
	switch {
	case p.isTest:
		return "test"
	case p.eventType == evGrab:
		return "grab"
	case p.eventType == evDownload:
		return "download"
	default:
		return "other"
	}
}

// handleTracked applies the Grab/Download event to the record (§4). It is
// idempotent: an event for an unknown key, or a terminal record, is a no-op.
func (h *Handler) handleTracked(ctx context.Context, p parsed) {
	rec, err := h.store.GetTitle(ctx, p.key)
	if err == store.ErrNotFound {
		// A webhook for a title we never requested (e.g. user grabbed manually).
		// Ack + ignore — we only track our own acquisitions (§6).
		return
	}
	if err != nil {
		h.log.Error("ingest: get title", "key", p.key, "err", err)
		return
	}

	now := h.now()
	var ev provision.Event
	switch p.eventType {
	case evGrab:
		// A release was grabbed → downloading; reset deadline to the shorter TTL.
		ev = provision.Event{Kind: provision.Grabbed, Deadline: now.Add(h.downloadingTTL)}
	case evDownload:
		// Import complete (the "Download" naming quirk, §6). Library is the
		// source of truth (invariant 4): confirm presence before available.
		itemID, present, lerr := h.lib.Lookup(ctx, providerKind(p.title), providerID(p.title), libraryMediaType(p.title.MediaType))
		if lerr != nil {
			// Library down: leave the record in flight; the reconciler re-checks
			// (missed-webhook backstop, §7). Don't mark available on faith.
			h.log.Warn("ingest: library lookup failed on import; deferring to reconciler", "key", p.key, "err", lerr)
			return
		}
		if !present {
			// Import fired but the item isn't visible yet (scan lag). Leave in
			// flight; the reconciler confirms later (§4 invariant 4).
			h.log.Info("ingest: import webhook but library not yet confirming; deferring", "key", p.key)
			return
		}
		ev = provision.Event{Kind: provision.LibraryConfirmed, LibraryID: itemID}
	default:
		return // untracked event type — no-op
	}

	next, emitted := provision.Apply(rec, ev, now)
	if next.State == rec.State && next.UpdatedAt.Equal(rec.UpdatedAt) {
		return // no-op transition
	}
	if err := h.store.UpsertTitle(ctx, next); err != nil {
		h.log.Error("ingest: persist transition", "key", p.key, "err", err)
		return
	}
	for _, de := range emitted {
		h.log.Info("provision event", "key", de.Key, "state", de.State)
		if h.emit != nil {
			h.emit.Emit(ctx, de) // → scheduler backfill + SSE (composition root fans out)
		}
	}
}

func (h *Handler) validSecret(r *http.Request) bool {
	got := r.URL.Query().Get("token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) == 1
}

// providerKind/providerID pick the library lookup key for a title (§6).
func providerKind(t provision.Title) library.ProviderKind {
	if t.MediaType == provision.Series && t.TVDBID > 0 {
		return library.TVDB
	}
	return library.TMDB
}

func providerID(t provision.Title) string {
	if t.MediaType == provision.Series && t.TVDBID > 0 {
		return itoa(t.TVDBID)
	}
	return itoa(t.TMDBID)
}
