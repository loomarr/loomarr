// Package backendtransition owns the durable workflow that separates preparing a playout backend
// from publishing it to the media server. Controller orders fleet convergence, target publication,
// cutover, and stale retirement around one versioned, system-owned checkpoint in the settings KV.
// Runtime mirrors only durable checkpoints for request-path transport gating. The row is runtime
// state, not a registry setting. Load and Save do not provide compare-and-swap: Controller
// serializes transitions within one process; cross-process leadership belongs to composition.
package backendtransition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

const (
	stateKey     = "system.playout_backend_transition"
	stateVersion = 1

	// BackendInternal identifies Loomarr's internal playout.
	BackendInternal = schedule.PlayoutBackendInternal
	// BackendTunarr identifies Tunarr-owned playout.
	BackendTunarr = "tunarr"
)

// ErrInvalidState reports a checkpoint that cannot safely direct publication.
// Load never replaces an invalid row with inferred state: the caller must repair it.
var ErrInvalidState = errors.New("backend transition: invalid state")

// State is the durable backend-publication checkpoint. Its fields are private so
// applied can advance only to the backend the caller has already marked prepared.
type State struct {
	applied  string
	prepared string
}

// These private ports narrow the existing persistence seam to exactly what loading
// and checkpointing need. store.Store satisfies them for both SQLite and Postgres.
type stateLoader interface {
	GetSetting(ctx context.Context, key string) (string, error)
	ListChannels(ctx context.Context) ([]store.Channel, error)
}

type stateWriter interface {
	SetSetting(ctx context.Context, key, value string) error
}

type stateStore interface {
	stateLoader
	stateWriter
}

type persistedState struct {
	Version  int    `json:"version"`
	Applied  string `json:"applied"`
	Prepared string `json:"prepared"`
}

type decodedState struct {
	Version  *int    `json:"version"`
	Applied  *string `json:"applied"`
	Prepared *string `json:"prepared"`
}

// Load returns the persisted checkpoint or initializes an absent one. A legacy
// install with any channel is assumed to have published Tunarr; only a genuinely
// empty fleet may initialize directly from the desired backend.
func Load(ctx context.Context, st stateStore, desired string) (State, error) {
	if err := validateBackend(desired); err != nil {
		return State{}, fmt.Errorf("%w: desired backend: %v", ErrInvalidState, err)
	}

	raw, err := st.GetSetting(ctx, stateKey)
	if err == nil {
		return decode(raw)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return State{}, fmt.Errorf("load backend transition state: %w", err)
	}

	channels, err := st.ListChannels(ctx)
	if err != nil {
		return State{}, fmt.Errorf("inspect fleet for backend transition initialization: %w", err)
	}
	initial := desired
	if len(channels) != 0 {
		initial = BackendTunarr
	}
	state := State{applied: initial}
	if err := Save(ctx, st, state); err != nil {
		return State{}, fmt.Errorf("initialize backend transition state: %w", err)
	}
	return state, nil
}

// Applied returns the backend whose tuner/listing pair is currently published.
func (s State) Applied() string { return s.applied }

// Prepared returns the backend awaiting publication, or "" in steady state.
func (s State) Prepared() string { return s.prepared }

// PublishedInternal reports whether internal device routes must be readable. They
// come up while internal is prepared, before the media server is pointed at them,
// and remain up while internal is the applied backend.
func (s State) PublishedInternal() bool {
	return s.applied == BackendInternal || s.prepared == BackendInternal
}

// MarkPrepared records that fleet convergence completed for backend. It does not
// publish that backend; Applied remains unchanged until PublishPrepared is called.
func (s State) MarkPrepared(backend string) (State, error) {
	if err := s.validate(); err != nil {
		return State{}, err
	}
	if err := validateBackend(backend); err != nil {
		return State{}, fmt.Errorf("%w: prepare backend: %v", ErrInvalidState, err)
	}
	s.prepared = backend
	return s, nil
}

// CancelPrepared abandons an unpublished preparation while preserving the backend
// currently applied to the media server. It is a no-op in steady state.
func (s State) CancelPrepared() (State, error) {
	if err := s.validate(); err != nil {
		return State{}, err
	}
	s.prepared = ""
	return s, nil
}

// PublishPrepared advances Applied to the already-prepared backend. Keeping this
// operation parameterless prevents callers from publishing an unprepared target.
func (s State) PublishPrepared() (State, error) {
	if err := s.validate(); err != nil {
		return State{}, err
	}
	if s.prepared == "" {
		return State{}, fmt.Errorf("%w: no prepared backend to publish", ErrInvalidState)
	}
	s.applied = s.prepared
	s.prepared = ""
	return s, nil
}

// Save validates and persists state as canonical versioned JSON.
func Save(ctx context.Context, st stateWriter, state State) error {
	if err := state.validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(persistedState{
		Version: stateVersion, Applied: state.applied, Prepared: state.prepared,
	})
	if err != nil {
		return fmt.Errorf("encode backend transition state: %w", err)
	}
	if err := st.SetSetting(ctx, stateKey, string(raw)); err != nil {
		return fmt.Errorf("store backend transition state: %w", err)
	}
	return nil
}

func decode(raw string) (State, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var persisted decodedState
	if err := decoder.Decode(&persisted); err != nil {
		return State{}, fmt.Errorf("%w: decode: %v", ErrInvalidState, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return State{}, fmt.Errorf("%w: decode: %v", ErrInvalidState, err)
	}
	if persisted.Version == nil || persisted.Applied == nil || persisted.Prepared == nil {
		return State{}, fmt.Errorf("%w: version, applied, and prepared fields are required", ErrInvalidState)
	}
	if *persisted.Version != stateVersion {
		return State{}, fmt.Errorf("%w: unsupported version %d", ErrInvalidState, *persisted.Version)
	}
	state := State{applied: *persisted.Applied, prepared: *persisted.Prepared}
	if err := state.validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s State) validate() error {
	if err := validateBackend(s.applied); err != nil {
		return fmt.Errorf("%w: applied: %v", ErrInvalidState, err)
	}
	if s.prepared != "" {
		if err := validateBackend(s.prepared); err != nil {
			return fmt.Errorf("%w: prepared: %v", ErrInvalidState, err)
		}
	}
	return nil
}

func validateBackend(backend string) error {
	switch backend {
	case BackendInternal, BackendTunarr:
		return nil
	default:
		return fmt.Errorf("unknown backend %q", backend)
	}
}
