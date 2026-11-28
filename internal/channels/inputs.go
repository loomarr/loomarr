package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// scheduleSettings is one pass's reading of the install settings an arrangement depends on. A
// reconcile or a forecast reads them once, so the deal, its dealt_from and the window it is held
// to all see the same values. The scheduled ledger and break planning read the rolling window
// live through Engine.RollingWindow.
type scheduleSettings struct {
	BreaksPerHour int                    // §10 global break density, before the channel's override
	BreakLength   time.Duration          // §10 global break length, before the channel's override
	Window        time.Duration          // §6.5 global rolling-window horizon
	Pending       schedule.PendingPolicy // the install's pending-title policy
	Calendar      schedule.Calendar      // the window grid's wall clock and holidays
	Backend       string                 // the install's playout backend (§9.1)
}

// settingsSnapshot reads the install settings once for a pass, resolving the playout backend.
func (e *Engine) settingsSnapshot(ctx context.Context) (scheduleSettings, error) {
	backend, err := e.playoutBackendFor(ctx)
	if err != nil {
		return scheduleSettings{}, err
	}
	return e.settingsOn(backend), nil
}

// settingsOn reads the install settings once for a pass whose playout backend is already known.
func (e *Engine) settingsOn(backend string) scheduleSettings {
	return scheduleSettings{
		BreaksPerHour: e.breaksPerHourFor(),
		BreakLength:   e.breakDurationFor(),
		Window:        e.defaultWindowFor(),
		Pending:       e.policy,
		Calendar:      e.calendarFor(),
		Backend:       backend,
	}
}

// rollingWindow is the channel's rolling window at `at` and the zone its grid is drawn in.
func (s scheduleSettings) rollingWindow(policy schedule.ChannelPolicy, at time.Time) (time.Duration, *time.Location) {
	ch := schedule.Channel{DefaultWindow: s.Window}.WithCalendar(s.Calendar)
	return schedule.ResolveWindow(ch, policy, at), s.Calendar.Zone
}

// fillerPool is what the channel's backend can air in a break.
type fillerPool struct {
	TunarrIDs  []string // the Tunarr filler list's program uuids; Tunarr channels only
	PlayableMs int64    // the playable media in the channel's pod; internal channels only
	Has        bool     // whether breaks are interleaved at all
}

// assemblyMode is who the inputs are assembled for. It decides exactly two things, each at its
// switch in assemble: whether the accepted window turns, and whether the Tunarr filler question
// records the §17 pod metric.
type assemblyMode uint8

const (
	assembleReconcile assemblyMode = iota // the pass that commits a deal
	assembleForecast                      // a read-only preview, guide forecast, diff or outlook
)

// scheduleInputs is everything one arrangement of a channel reads. Reconcile and the forecast
// build it through assemble, so the two cannot ask different questions of the same channel.
type scheduleInputs struct {
	Channel         schedule.Channel // the domain channel with every derived field set
	Lineup          []schedule.LineupEntry
	Policy          schedule.ChannelPolicy
	At              time.Time
	PlaysInternally bool
	Filler          fillerPool
	Settings        scheduleSettings
	TurnAnchor      time.Time // the anchor the next window starts from; the accepted anchor until it turns
	Turned          bool      // the accepted window has ended and a new one starts at TurnAnchor
	// Scheduled is the ledger rows the accepted cycle aired, watched or not (#1677); reconcile
	// only. The commit writes them once its CAS proves this cycle was still the accepted one.
	Scheduled []channeldatastore.ScheduledAiring
}

// assemble builds a channel's arrangement inputs at `at` from one settings snapshot. Reconcile
// heals the lineup before it assembles; the forecast reads the lineup as the last reconcile left it.
func (e *Engine) assemble(ctx context.Context, ch channelstore.Channel, at time.Time, s scheduleSettings, mode assemblyMode) scheduleInputs {
	in := scheduleInputs{
		Lineup: ch.Lineup, Policy: ch.Policy, At: at, Settings: s, TurnAnchor: ch.PlayoutAnchor,
		PlaysInternally: schedule.PlaysInternally(ch.Policy, s.Backend),
	}
	in.Filler = e.fillerPoolFor(ctx, ch, in.PlaysInternally, mode)

	// Breaks are only interleaved when the backend has clips to fill them (§10): a gap with no
	// clips is a promise of commercials nobody can keep.
	d := ch.Channel
	d.BreaksPerHour = BreaksPerHourFor(ch.Policy, in.Filler.Has, s.BreaksPerHour)
	d.BreakDurationMs = BreakDurationFor(ch.Policy, s.BreakLength).Milliseconds()
	d.DefaultWindow = s.Window
	d = d.WithCalendar(s.Calendar)
	d.DealOrigin = schedule.ResolveDealOrigin(ch.CreatedAt, ch.PlayoutAnchor) // #1676

	var accepted []schedule.Slot
	if in.PlaysInternally {
		// Rolling-window carry-over (#1675) is reconcile's alone: it is the pass that moves the
		// anchor. The forecast arranges the window `at` falls in. Tunarr loops the list it was
		// given from its own clock and cannot be told where a window starts (#1691).
		if mode == assembleReconcile {
			window, zone := s.rollingWindow(ch.Policy, at)
			d.WindowOpened, in.TurnAnchor, in.Turned = timeline.WindowTurn(ch.Desired, ch.PlayoutAnchor, window, zone, at)
			in.Scheduled = scheduledToRecord(ch, at, window, zone) // #1677: what the accepted cycle aired
		}
		accepted = ch.Desired // #1677: the accepted cycle's programmes count as aired
	}
	d.LastAired = e.lastAiredFor(ctx, d, ch.Policy, at, accepted, ch.PlayoutAnchor)
	d.NaturalBreaks = e.naturalBreaksFor(ctx, ch.Policy, in.PlaysInternally)
	d.PinnedCuts = pinnedCutsAt(ch, in.PlaysInternally, at)
	in.Channel = d
	return in
}

// fillerPoolFor asks the channel's backend whether it has clips to air in a break. The question
// depends on the backend, never on who asks: internal playout airs local files and needs no
// Tunarr uuid, and Tunarr airs only the clips its filler list can reference. Only reconcile's
// question records the §17 pod metric, so guide polls do not inflate it.
func (e *Engine) fillerPoolFor(ctx context.Context, ch channelstore.Channel, playsInternally bool, mode assemblyMode) fillerPool {
	if e.pods == nil {
		return fillerPool{}
	}
	seed, selection := PodSeed(ch.ID), SelectionForChannel(ch)
	if playsInternally {
		ms := e.pods.PlayableDurationMs(ctx, ch.ID, seed, selection)
		return fillerPool{PlayableMs: ms, Has: ms > 0}
	}
	var ids []string
	switch mode {
	case assembleReconcile:
		if list, ok := e.pods.BuildFillerList(ctx, ch.ID, seed, selection); ok {
			ids = list
		}
	case assembleForecast:
		ids = e.pods.TunarrFillerIDs(ctx, ch.ID, seed, selection)
	}
	return fillerPool{TunarrIDs: ids, Has: len(ids) > 0}
}

// compute arranges the channel from its inputs. An internal channel's breaks end with its pod's
// playable media.
func (in scheduleInputs) compute(ctx context.Context, avail schedule.Availability) (schedule.DesiredLineup, error) {
	desired, err := schedule.ComputeDesiredAtCtx(ctx, in.Channel, in.Lineup, avail, in.Settings.Pending, in.Policy, in.At)
	if err != nil {
		return desired, err
	}
	if in.PlaysInternally {
		desired.Slots = capCommercialBreaks(desired.Slots, in.Filler.PlayableMs)
	}
	return desired, nil
}
