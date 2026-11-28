package channels_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// updateScheduleGoldens rewrites testdata/schedule-goldens. The goldens pin what reconcile
// commits and what the forecast shows for six fixed channels. dealt_from is "<edited>.<dealt>": a
// changed <edited> half re-deals every internal channel after the programme on air at its first
// sweep after deploy, and a changed <dealt> half re-deals every unbounded one. A diff in either is
// a maintainer decision, never a re-record that makes a test pass.
var updateScheduleGoldens = flag.Bool("update-schedule-goldens", false, "rewrite testdata/schedule-goldens")

// equivalenceStart is the fixed clock every equivalence cell starts at.
var equivalenceStart = time.Unix(1_800_000_000, 0).UTC()

// fillerPool names one shape of filler catalog the pool predicates must agree on.
type fillerPool string

const (
	poolNone       fillerPool = "none"        // no clips at all
	poolBumperOnly fillerPool = "bumper-only" // station material, which is not a pool on its own
	poolUUID       fillerPool = "uuid"        // commercials with a file and a Tunarr program uuid
	poolPathOnly   fillerPool = "path-only"   // commercials with a file and no Tunarr uuid
)

func (p fillerPool) catalog() fixedClipCatalog {
	ad := func(n int, uuid string) clips.Clip {
		return clips.Clip{
			Hash: fmt.Sprintf("ad-%d", n), Path: fmt.Sprintf("ad-%d.mp4", n), TunarrProgramID: uuid,
			Name: fmt.Sprintf("Ad %d", n), Role: clips.Commercial, DurationMs: 20_000,
			Era: 1992, Audience: clips.General,
		}
	}
	switch p {
	case poolBumperOnly:
		return fixedClipCatalog{{
			Hash: "bumper", Path: "bumper.mp4", TunarrProgramID: "bumper-program",
			Name: "Bumper", Role: clips.Bumper, DurationMs: 10_000, Era: 1992, Audience: clips.General,
		}}
	case poolUUID:
		return fixedClipCatalog{ad(1, "program-1"), ad(2, "program-2")}
	case poolPathOnly:
		return fixedClipCatalog{ad(1, ""), ad(2, "")}
	default:
		return nil
	}
}

// equivalenceCell is one channel shape: the backend that plays it, its filler catalog, whether
// it already has an accepted cycle, and the install's rolling window (0 is the whole run).
type equivalenceCell struct {
	backend  string
	pool     fillerPool
	accepted bool
	window   time.Duration
}

func (c equivalenceCell) name() string {
	history := "fresh"
	if c.accepted {
		history = "accepted"
	}
	window := "unbounded"
	if c.window > 0 {
		window = "bounded"
	}
	return fmt.Sprintf("%s/%s/%s/%s", c.backend, c.pool, history, window)
}

// equivalenceRun is a cell reconciled once at equivalenceStart and, for an accepted cell, again
// ninety minutes later, when the first cycle is the accepted one, and five hours later, after the
// bounded window has turned. Each step keeps what reconcile
// committed and what the forecast shows at that instant.
type equivalenceRun struct {
	steps []equivalenceStep
}

type equivalenceStep struct {
	At        time.Time       `json:"at"`
	Committed []schedule.Slot `json:"committed"`
	DealtFrom string          `json:"dealtFrom"`
	Forecast  []schedule.Slot `json:"forecast"`
}

func runEquivalenceCell(t *testing.T, c equivalenceCell) equivalenceRun {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	now := equivalenceStart
	var prog tunarr.Programmer
	if c.backend == schedule.PlayoutBackendTunarr {
		prog = tunarrtest.NewTunarr()
	}
	pods := pod.NewAdapter(c.pool.catalog(), nil, func() pod.Policy {
		return pod.Policy{PodMax: 4, BreakDurationMs: 60_000}
	}, testkit.Logger())
	avail := mapAvail{}
	e := channels.New(st, prog, avail, nil, channels.Config{
		ReconcileTTL:         10 * time.Minute,
		BreaksPerHour:        2,
		BreakDuration:        time.Minute,
		ResolveDefaultWindow: func() time.Duration { return c.window },
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return c.backend, nil
		},
	}, func() time.Time { return now }, testkit.Logger()).WithPods(pods)
	// Six hours of programmes outlast the bounded window, so a bounded deal is a slice of the deck
	// and its window turns before the last step.
	seeded := channelstore.Channel{CreatedAt: equivalenceStart}
	seeded.ID, seeded.Name, seeded.Number = "eq", "Equivalence", 7
	seeded.Strategy, seeded.Status = schedule.Sequential, schedule.StatusBuilding
	for i, key := range []string{
		"movie:tmdb:90000001", "movie:tmdb:90000002", "movie:tmdb:90000003",
		"movie:tmdb:90000004", "movie:tmdb:90000005", "movie:tmdb:90000006",
	} {
		avail[provision.Key(key)] = fmt.Sprintf("lib-%d", i+1)
		seeded.Lineup = append(seeded.Lineup, entry(key, fmt.Sprintf("Title %d", i+1)))
	}
	if _, err := st.SaveChannel(ctx, seeded); err != nil {
		t.Fatal(err)
	}

	instants := []time.Time{equivalenceStart}
	if c.accepted {
		instants = append(instants, equivalenceStart.Add(90*time.Minute), equivalenceStart.Add(5*time.Hour))
	}
	var run equivalenceRun
	for _, at := range instants {
		now = at
		if err := e.Reconcile(ctx, "eq"); err != nil {
			t.Fatalf("reconcile at %v: %v", at, err)
		}
		ch, err := st.GetChannel(ctx, "eq")
		if err != nil {
			t.Fatal(err)
		}
		_, forecast, _, _, err := e.CyclePreview(ctx, "eq", at)
		if err != nil {
			t.Fatalf("forecast at %v: %v", at, err)
		}
		run.steps = append(run.steps, equivalenceStep{
			At: at, Committed: ch.Desired, DealtFrom: ch.DealtFrom, Forecast: forecast,
		})
	}
	return run
}

func equivalenceCells() []equivalenceCell {
	var cells []equivalenceCell
	for _, backend := range []string{schedule.PlayoutBackendInternal, schedule.PlayoutBackendTunarr} {
		for _, pool := range []fillerPool{poolNone, poolBumperOnly, poolUUID, poolPathOnly} {
			for _, accepted := range []bool{false, true} {
				for _, window := range []time.Duration{4 * time.Hour, 0} {
					cells = append(cells, equivalenceCell{backend, pool, accepted, window})
				}
			}
		}
	}
	return cells
}

// forecastMatchesCommit reports whether the forecast at a reconcile's instant must show exactly
// the slots that reconcile committed.
//
// A Tunarr channel diverges in two known cells, because reconcile asks for the uuid filler list
// while the forecast asks HasPool. With clips that have files but no Tunarr uuid, reconcile airs
// no breaks and the forecast shows them. With station material only, reconcile attaches it as the
// filler list and airs breaks, and the forecast shows none.
func forecastMatchesCommit(c equivalenceCell, step int) bool {
	return !tunarrPoolDiverges(c)
}

func tunarrPoolDiverges(c equivalenceCell) bool {
	return c.backend == schedule.PlayoutBackendTunarr && (c.pool == poolPathOnly || c.pool == poolBumperOnly)
}

// committedBreaks reports whether reconcile airs breaks for a cell. Every catalog with a playable
// file gives an internal channel breaks, station material included; a Tunarr channel needs a clip
// with a Tunarr uuid.
func committedBreaks(c equivalenceCell) bool {
	if c.pool == poolNone {
		return false
	}
	return c.backend == schedule.PlayoutBackendInternal || c.pool != poolPathOnly
}

// TestReconcileForecastEquivalence holds the forecast to what reconcile commits across backend ×
// filler pool × history × window. The guide, the schedule diff and the proposal outlook all read
// the forecast, so a divergence shows a viewer a schedule that never airs.
func TestReconcileForecastEquivalence(t *testing.T) {
	for _, c := range equivalenceCells() {
		t.Run(c.name(), func(t *testing.T) {
			run := runEquivalenceCell(t, c)
			for i, step := range run.steps {
				if !forecastMatchesCommit(c, i) {
					continue
				}
				if got, want := slotsJSON(t, step.Forecast), slotsJSON(t, step.Committed); got != want {
					t.Errorf("step %d at %v: forecast differs from the committed lineup\nforecast:  %s\ncommitted: %s",
						i, step.At, got, want)
				}
			}
			breaks := fillerSlotCount(run.steps[0].Committed)
			if wantBreaks := committedBreaks(c); (breaks > 0) != wantBreaks {
				t.Errorf("committed %d break slots, want breaks=%v", breaks, wantBreaks)
			}
		})
	}
}

// scheduleGoldenCells are the six channels whose committed lineup, dealt_from and forecast are
// pinned byte for byte.
var scheduleGoldenCells = []equivalenceCell{
	{schedule.PlayoutBackendInternal, poolUUID, true, 4 * time.Hour},
	{schedule.PlayoutBackendInternal, poolPathOnly, true, 0},
	{schedule.PlayoutBackendInternal, poolNone, true, 4 * time.Hour},
	{schedule.PlayoutBackendTunarr, poolUUID, true, 4 * time.Hour},
	{schedule.PlayoutBackendTunarr, poolPathOnly, true, 4 * time.Hour},
	{schedule.PlayoutBackendTunarr, poolNone, true, 0},
}

// TestScheduleGoldens pins today's committed lineups, dealt_from values and forecasts. See
// updateScheduleGoldens before changing a file under testdata/schedule-goldens.
func TestScheduleGoldens(t *testing.T) {
	for _, c := range scheduleGoldenCells {
		t.Run(c.name(), func(t *testing.T) {
			run := runEquivalenceCell(t, c)
			got, err := json.MarshalIndent(run.steps, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", "schedule-goldens", fmt.Sprintf("%s-%s-%s.json",
				c.backend, c.pool, map[bool]string{true: "bounded", false: "unbounded"}[c.window > 0]))
			if *updateScheduleGoldens {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(got, want) {
				gotLines, wantLines := bytes.Split(got, []byte("\n")), bytes.Split(want, []byte("\n"))
				line := 0
				for line < len(gotLines) && line < len(wantLines) && bytes.Equal(gotLines[line], wantLines[line]) {
					line++
				}
				t.Errorf("%s differs from what reconcile and the forecast produce, first at line %d", path, line+1)
			}
		})
	}
}

func slotsJSON(t *testing.T, slots []schedule.Slot) string {
	t.Helper()
	blob, err := json.Marshal(slots)
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}
