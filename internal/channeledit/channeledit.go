// Package channeledit is the one write path for an operator's channel changes (§7): creating a
// hand-made or approved-intent channel, and every edit after it — the PATCH form, an icon or
// watermark upload, a detach. Each change is a function over the channel; the service wraps it in
// the rules every edit must share, so a route cannot forget one: the `building` tuner-list
// freshness signal, the due-now reconcile deadline, the UpdatedAt stamp, validation, the
// optimistic revision, the internal-playout lifecycle, and the best-effort reconcile.
//
// Proposal approval is NOT here: it plans its channel write through `binder` inside the approval
// transaction. This package borrows binder's answers (lineup, policy, number in use, id scheme)
// so the two paths cannot disagree about them.
package channeledit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/activity"
	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// Refusals. A caller maps each to the message it shows; any other error is unexpected.
var (
	ErrNotFound    = errors.New("channel not found")
	ErrStale       = errors.New("channel changed since it was read")
	ErrNumberInUse = errors.New("channel number in use")
	ErrIDTaken     = errors.New("channel id already exists")
	ErrIntentBound = errors.New("approved suggestion already bound to a channel")
	// ErrClaimed is a number or intent another writer took between the check and the save.
	ErrClaimed          = errors.New("channel number or approved suggestion claimed by another channel")
	ErrInvalid          = errors.New("invalid channel")
	ErrInvalidSeries    = errors.New("single-series channel needs a series key")
	ErrLineupUnresolved = errors.New("approved suggestion's lineup could not be resolved")
	ErrPolicyUnresolved = errors.New("approved suggestion's policy could not be resolved")
)

// Store is the channel persistence the service writes through.
type Store interface {
	GetChannel(ctx context.Context, id string) (store.Channel, error)
	GetChannelByIntentRef(ctx context.Context, intentRef string) (store.Channel, error)
	GetChannelByNumber(ctx context.Context, number int) (store.Channel, error)
	SaveChannel(ctx context.Context, ch store.Channel) (store.Channel, error)
}

// Binder is the approval path's answers, shared so explicit create and approval agree.
type Binder interface {
	LineupFromIntent(ctx context.Context, intentRef string) ([]schedule.LineupEntry, error)
	PolicyFromIntent(ctx context.Context, intentRef string) (schedule.ChannelPolicy, error)
	// NumberInUse asks Loomarr's store AND Tunarr (design §2: a number must be free in Tunarr too).
	NumberInUse(ctx context.Context, number int) (bool, error)
}

// Reconciler converges one channel on its selected playout backend.
type Reconciler interface {
	Reconcile(ctx context.Context, channelID string) error
}

// Backend is the applied playout backend the edit is judged against, read once per request so
// the caller renders the result against the same snapshot.
type Backend struct {
	Applied           string
	PublishedInternal bool
}

// Service applies operator channel changes. Binder, Reconciler, Lifecycle, Log and Activity are
// optional; Store is required.
type Service struct {
	Store Store
	// Binder nil ⇒ hand-made channels only, and the number check asks the store alone (a
	// store-only install still gets the Loomarr half of the answer rather than none).
	Binder     Binder
	Reconciler Reconciler
	// Lifecycle runs after a committed edit that changed whether the channel is playable on the
	// internal transport (stop sessions, rescan the tuner). It is called only on a change.
	Lifecycle func(ctx context.Context, channelID string, wasPlayable, isPlayable bool)
	Log       *slog.Logger
	Activity  *activity.Recorder
}

// NewChannel is a create request. ID is optional (minted with binder's `ch_…` scheme when empty,
// so a hand-made channel is indistinguishable from an approved one).
type NewChannel struct {
	ID, Name, Group, Logo string
	Number                int
	Strategy              schedule.Strategy
	IntentRef             string
	// Series makes a hand-made single-series channel; ignored when IntentRef is set.
	Series *SeriesScope
}

// SeriesScope is one series with an optional season range (0 = unbounded).
type SeriesScope struct {
	Key                  provision.Key
	Title                string
	SeasonMin, SeasonMax int
}

// Create validates and saves a new channel, then kicks its first reconcile.
func (s *Service) Create(ctx context.Context, in NewChannel) (store.Channel, error) {
	ch := store.Channel{
		ID: in.ID, Name: in.Name, Number: in.Number, Group: in.Group, Logo: in.Logo,
		Strategy: in.Strategy, IntentRef: in.IntentRef, Status: schedule.StatusBuilding,
	}
	if ch.ID == "" {
		ch.ID = binder.NewChannelID()
	}
	// A non-empty intent has exactly one local channel. Approval enforces this at the database
	// boundary; surface it here as a refusal instead of a unique-index failure.
	if ch.IntentRef != "" {
		if _, err := s.Store.GetChannelByIntentRef(ctx, ch.IntentRef); err == nil {
			return store.Channel{}, ErrIntentBound
		} else if !errors.Is(err, store.ErrNotFound) {
			return store.Channel{}, err
		}
	}
	// Bind the approved proposal's lineup and grounded policy (§7/§9, programming-design §8) so
	// enforcement applies from the first reconcile. No intent ⇒ empty lineup, built-in defaults.
	if ch.IntentRef != "" && s.Binder != nil {
		lineup, err := s.Binder.LineupFromIntent(ctx, ch.IntentRef)
		if err != nil {
			return store.Channel{}, fmt.Errorf("%w: %w", ErrLineupUnresolved, err)
		}
		policy, err := s.Binder.PolicyFromIntent(ctx, ch.IntentRef)
		if err != nil {
			return store.Channel{}, fmt.Errorf("%w: %w", ErrPolicyUnresolved, err)
		}
		ch.Lineup, ch.Policy = lineup, policy
	}
	if ch.IntentRef == "" && in.Series != nil {
		if !in.Series.Key.IsSeries() {
			return store.Channel{}, ErrInvalidSeries
		}
		ch.Lineup = []schedule.LineupEntry{{
			Key: in.Series.Key, Title: in.Series.Title,
			SeasonMin: in.Series.SeasonMin, SeasonMax: in.Series.SeasonMax,
		}}
	}
	// Due NOW, so the sweep owns this channel from the moment it exists (§9 V54), the same stamp
	// approval applies: the reconcile below is best-effort, and without a deadline the retry that
	// is supposed to cover it never comes.
	ch.ReconcileDeadline = time.Now().UTC()
	if err := ch.Validate(); err != nil {
		return store.Channel{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if _, err := s.Store.GetChannel(ctx, ch.ID); err == nil {
		return store.Channel{}, ErrIDTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Channel{}, err
	}
	if err := s.numberFree(ctx, ch.Number); err != nil {
		return store.Channel{}, err
	}
	saved, err := s.Store.SaveChannel(ctx, ch)
	if err != nil {
		if errors.Is(err, store.ErrChannelConflict) {
			return store.Channel{}, ErrClaimed
		}
		return store.Channel{}, err
	}
	// Go live immediately (§9 "never dead air"). A failure leaves the channel `building` for the
	// sweep; it does not fail creation.
	if s.Reconciler != nil {
		if err := s.Reconciler.Reconcile(ctx, saved.ID); err != nil {
			// ERROR + the consequence (§9 V54): the channel exists but has not converged.
			s.log().Error("initial channel reconcile FAILED — it is not live yet; the next channel sweep will retry",
				"channel", saved.ID, "name", saved.Name, "number", saved.Number, "err", err)
			s.Activity.Error(ctx, "channel.reconcile", saved.ID,
				fmt.Sprintf("%q couldn't be brought live on its selected playout backend yet: %v. Loomarr will keep retrying.", saved.Name, err))
		}
	}
	return s.reread(ctx, saved.ID)
}

// maxRetries bounds the re-read-and-reapply loop of an Edit with no revision.
const maxRetries = 4

// Edit applies change to the channel and saves it under the shared edit rules. A change returns
// an error to refuse; that error is returned as is.
//
// revision > 0 is the revision the operator's edit was based on: a mismatch, or a concurrent save,
// is ErrStale and nothing is written — the only honest recovery for a whole-form edit is to
// refresh. revision 0 applies the change to the latest channel and re-reads and re-applies it when
// another write lands first, so change must be safe to run more than once.
func (s *Service) Edit(ctx context.Context, backend Backend, id string, revision int64, change func(*store.Channel) error) (store.Channel, error) {
	for attempt := 1; ; attempt++ {
		saved, err := s.editOnce(ctx, backend, id, revision, change)
		if !errors.Is(err, ErrStale) || revision > 0 || attempt == maxRetries {
			if err != nil {
				return store.Channel{}, err
			}
			return saved, nil
		}
	}
}

func (s *Service) editOnce(ctx context.Context, backend Backend, id string, revision int64, change func(*store.Channel) error) (store.Channel, error) {
	before, err := s.Store.GetChannel(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Channel{}, ErrNotFound
	} else if err != nil {
		return store.Channel{}, err
	}
	if revision > 0 && revision != before.Revision {
		return store.Channel{}, ErrStale
	}
	ch := before
	if err := change(&ch); err != nil {
		return store.Channel{}, err
	}
	// No "except me" for a number: Tunarr's list carries no identity to exclude by, so ask only
	// when the number actually changes.
	if ch.Number != before.Number {
		if err := s.numberFree(ctx, ch.Number); err != nil {
			return store.Channel{}, err
		}
	}
	applyEditRules(before, &ch, backend)
	if err := ch.Validate(); err != nil {
		return store.Channel{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	ch.UpdatedAt = time.Now().Unix() // SaveChannel does not stamp this
	saved, err := s.Store.SaveChannel(ctx, ch)
	switch {
	case errors.Is(err, store.ErrChannelStale):
		return store.Channel{}, ErrStale
	case errors.Is(err, store.ErrNotFound):
		return store.Channel{}, ErrNotFound
	case errors.Is(err, store.ErrChannelConflict):
		return store.Channel{}, ErrClaimed
	case err != nil:
		return store.Channel{}, err
	}
	wasPlayable := schedule.InternalTransportPlayable(before.Status, before.Policy, backend.PublishedInternal)
	isPlayable := schedule.InternalTransportPlayable(saved.Status, saved.Policy, backend.PublishedInternal)
	if wasPlayable != isPlayable && s.Lifecycle != nil {
		s.Lifecycle(ctx, saved.ID, wasPlayable, isPlayable)
	}
	// Auto-reconcile so the edit reaches the backend with no user action (§9 "self-maintaining").
	// Best-effort and skipped while paused: the edit is durable, and the sweep is the guarantee.
	if saved.Status.Reconcilable() && s.Reconciler != nil {
		if err := s.Reconciler.Reconcile(ctx, saved.ID); err != nil {
			s.log().Warn("reconcile after channel edit failed (sweep will retry)", "channel", saved.ID, "err", err)
		}
	}
	return s.reread(ctx, saved.ID)
}

// applyEditRules stamps the reconcile signals an edit implies. `building` is the durable tuner-list
// freshness signal reconcile consumes, so it is NOT stamped onto every edit: a lineup or policy
// change on a settled channel changes guide data, not the tuner's channel list. Identity and
// effective-backend changes, and an empty internal channel starting to materialize, change
// membership or M3U identity and keep the stronger signal.
func applyEditRules(before store.Channel, ch *store.Channel, backend Backend) {
	if !ch.Status.Reconcilable() {
		return
	}
	identityChanged := before.Name != ch.Name || before.Number != ch.Number ||
		before.Group != ch.Group || before.Logo != ch.Logo
	backendChanged := schedule.PlaysInternally(before.Policy, backend.Applied) != schedule.PlaysInternally(ch.Policy, backend.Applied)
	materializingInternal := before.Status == schedule.StatusEmpty && schedule.PlaysInternally(ch.Policy, backend.Applied)
	if identityChanged || backendChanged || materializingInternal {
		ch.Status = schedule.StatusBuilding
	}
	// Zero is the store's explicit due-now value, so a failed immediate reconcile cannot leave the
	// edit waiting behind an old future deadline.
	ch.ReconcileDeadline = time.Time{}
}

// numberFree refuses a number already used in Loomarr or, with a binder, in Tunarr.
func (s *Service) numberFree(ctx context.Context, number int) error {
	if s.Binder != nil {
		inUse, err := s.Binder.NumberInUse(ctx, number)
		if err != nil {
			return err
		}
		if inUse {
			return ErrNumberInUse
		}
		return nil
	}
	_, err := s.Store.GetChannelByNumber(ctx, number)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil
	case err != nil:
		return err
	default:
		return ErrNumberInUse
	}
}

// reread returns the channel as stored after the reconcile, which may have advanced it.
func (s *Service) reread(ctx context.Context, id string) (store.Channel, error) {
	ch, err := s.Store.GetChannel(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Channel{}, ErrNotFound
	}
	return ch, err
}

func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}
