package growth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/rating"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
)

// The numbers the maintainer chose from the measured install (#4333).
const (
	Margin       = 3    // years either side of the seeds' span
	Bar          = 0.5  // genre share a clear fit reaches both ways (see Line.Classify)
	MaxShowShare = 0.15 // the largest share of a day one show may take: 7 shows
)

// DailyAirtime is the programme time a day carries, breaks excluded, for sizing a channel.
const DailyAirtime = 20 * time.Hour

// FillStore is the slice of the store the fill reads.
type FillStore interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	ChannelHasUnfinishedJob(ctx context.Context, channelID string) (bool, error)
	LibraryTitles(ctx context.Context, since time.Time) ([]librarystore.LibraryTitle, error)
	LibraryItemFacts(ctx context.Context, ids []string) (map[string]librarystore.LibraryItemFacts, error)
	SeriesEpisodeTotals(ctx context.Context) (map[string]librarystore.SeriesEpisodes, error)
	GrowthReviews(ctx context.Context, channelID string) (map[provision.Key]channelstore.GrowthReview, error)
}

// Recorder records a ready-made proposal for an existing channel (workflow.Workflow).
type Recorder interface {
	RecordForChannel(ctx context.Context, channelID string, intent proposals.Intent, prop proposals.Proposal) (workflow.CommittedProposal, error)
}

// Approver approves a proposal (accept.Approver).
type Approver interface {
	Approve(ctx context.Context, p proposalstore.Proposal, edit *accept.ApprovalEdit, approvedBy string) (accept.ApprovalResult, error)
}

// Fill is the nightly `channel-growth` job (#4333): each channel that grows takes, at midnight,
// the library titles inside its line until it reaches its target. The titles join through a
// proposal approved under the channel's auto-curate grant, so the binder stays the only lineup
// writer and unions them onto the lineup as one batch that can be undone as one.
type Fill struct {
	store    FillStore
	recorder Recorder
	approver Approver
	hasModel func() bool
	now      func() time.Time
	log      *slog.Logger
}

// NewFill wires the job. hasModel reports, at each run, whether a model is configured: where one
// is, an unclear fit waits for its review rather than joining by itself (decision 8).
func NewFill(st FillStore, rec Recorder, ap Approver, hasModel func() bool, now func() time.Time, log *slog.Logger) *Fill {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Fill{store: st, recorder: rec, approver: ap, hasModel: hasModel, now: now, log: log}
}

// libraryTitle is a library series or film as the fill reads it.
type libraryTitle struct {
	candidate     Candidate
	name          string
	libraryItemID string
	tmdb, tvdb    int
	runtimeMs     int64
}

// library is every playable series and film, reachable by each key form a lineup may use.
type library struct {
	titles []libraryTitle
	byKey  map[provision.Key]int
}

// Run fills every channel that grows. One channel's failure is logged and skipped; a run in which
// every attempted channel failed returns those failures.
func (f *Fill) Run(ctx context.Context) (grown int, err error) {
	lib, err := loadLibrary(ctx, f.store)
	if err != nil {
		return 0, err
	}
	chans, err := f.store.ListChannels(ctx)
	if err != nil {
		return 0, fmt.Errorf("growth: list channels: %w", err)
	}
	batch := "growth-" + f.now().UTC().Format("2006-01-02")
	withUnclear := !f.hasModel()
	var failures []error
	for _, ch := range chans {
		if !ch.Status.Reconcilable() || !ch.Policy.Grows() {
			continue
		}
		if !ch.FromRequest {
			// The approval path binds only to a channel made from a request; a channel made by
			// hand grows once growth has a writer of its own for it (#4333).
			f.log.Debug("growth: channel made by hand is not grown yet", "channel", ch.ID)
			continue
		}
		added, err := f.fillChannel(ctx, ch, lib, batch, withUnclear)
		if err != nil {
			f.log.Warn("growth: channel not filled", "channel", ch.ID, "err", err)
			failures = append(failures, fmt.Errorf("channel %s: %w", ch.ID, err))
			continue
		}
		if added > 0 {
			grown++
			f.log.Info("growth: titles joined", "channel", ch.ID, "batch", batch, "titles", added)
		}
	}
	if grown == 0 && len(failures) > 0 {
		return 0, errors.Join(failures...)
	}
	return grown, nil
}

func (f *Fill) fillChannel(ctx context.Context, ch channelstore.Channel, lib library, batch string, withUnclear bool) (int, error) {
	if busy, err := f.store.ChannelHasUnfinishedJob(ctx, ch.ID); err != nil {
		return 0, err
	} else if busy {
		return 0, nil // a proposal is already in flight for this channel; the next night tries again
	}
	reviews, err := f.store.GrowthReviews(ctx, ch.ID)
	if err != nil {
		return 0, err
	}
	picks, items := plan(ch, lib, reviews, withUnclear)
	if len(picks) == 0 {
		return 0, nil
	}
	intent, err := suggest.IntentOfChannel(ch)
	if err != nil && !errors.Is(err, suggest.ErrChannelIntentUnavailable) {
		return 0, err
	}
	committed, err := f.recorder.RecordForChannel(ctx, ch.ID, intent, proposals.Proposal{
		Lineup: items, Policy: ch.Policy, Growth: batch,
	})
	if err != nil {
		return 0, fmt.Errorf("record growth proposal: %w", err)
	}
	if _, err := f.approver.Approve(ctx, proposalstore.Proposal{
		ID: committed.ID, JobID: committed.JobID, Status: "submitted", Body: committed.Proposal,
		CreatedAt: committed.CreatedAt, UpdatedAt: committed.CreatedAt,
	}, nil, proposalstore.AutoCuratedBy); err != nil {
		return 0, fmt.Errorf("approve growth proposal: %w", err)
	}
	return len(picks), nil
}

// channelView is a channel as growth sees it: the line its seeds draw, what it carries and needs,
// and the library titles its scope, ceiling and holidays admit that it does not carry yet.
type channelView struct {
	line       Line
	carried    Carried
	need       Need
	candidates []Candidate
	byKey      map[provision.Key]int // a candidate's library title
}

// plan is what joins ch tonight, as picks and as the proposal items that carry them.
func plan(ch channelstore.Channel, lib library, reviews map[provision.Key]channelstore.GrowthReview, withUnclear bool) ([]Pick, []proposals.ProposalItem) {
	v := view(ch, lib)
	if v.line.Fixed {
		return nil, nil
	}
	picks := Plan(v.line, v.candidates, v.carried, TargetFor(v.need, MaxShowShare), Bar, ch.Policy.Declined,
		judgedAgainst(v.line, reviews), withUnclear)
	items := make([]proposals.ProposalItem, 0, len(picks))
	for _, p := range picks {
		t := lib.titles[v.byKey[p.Key]]
		mt := provision.Movie
		if t.candidate.Title.Kind == Series {
			mt = provision.Series
		}
		confidence := 0.9
		if p.Fit == Unclear {
			confidence = 0.6
		}
		items = append(items, proposals.ProposalItem{MediaType: mt, TMDBID: t.tmdb, TVDBID: t.tvdb, Name: t.name,
			Year: t.candidate.Title.Year, InLibrary: true, LibraryItemID: t.libraryItemID,
			OfficialRating: string(t.candidate.Title.Rating), Genres: t.candidate.Title.Genres,
			Confidence: confidence, Source: "growth"})
	}
	return picks, items
}

func view(ch channelstore.Channel, lib library) channelView {
	carriedKeys := map[int]bool{}
	var seeds []Title
	var carried Carried
	var seriesMs, filmMs int64
	for _, e := range ch.Lineup {
		i, held := lib.byKey[e.Key]
		if held {
			carriedKeys[i] = true
		}
		if e.Grown == "" {
			seeds = append(seeds, Title{Key: e.Key, Kind: kindOfKey(e.Key), Year: e.Year, Rating: e.OfficialRating,
				Genres: e.Genres, Franchise: max(e.CollectionID, 0), Airable: held})
		}
		if !held {
			continue
		}
		t := lib.titles[i]
		if t.candidate.Title.Kind == Series {
			carried.Shows++
			carried.Episodes += t.candidate.Episodes
			seriesMs += t.runtimeMs
		} else {
			carried.Films++
			filmMs += t.runtimeMs
		}
	}
	line := DrawLine(seeds, Margin)
	if line.Fixed {
		return channelView{line: line}
	}
	need := Need{DailyAirtime: DailyAirtime}
	if total := seriesMs + filmMs; total > 0 {
		need.FilmShare = float64(filmMs) / float64(total)
	}
	if carried.Episodes > 0 {
		need.MeanEpisode = time.Duration(seriesMs/int64(carried.Episodes)) * time.Millisecond
	}
	if carried.Films > 0 {
		need.MeanFilm = time.Duration(filmMs/int64(carried.Films)) * time.Millisecond
	}

	// Only what the channel's own scope and ceiling admit is a candidate: growth narrows them.
	var entries []schedule.LineupEntry
	byEntry := map[provision.Key]int{}
	for i, t := range lib.titles {
		if carriedKeys[i] {
			continue
		}
		ct := t.candidate.Title
		entries = append(entries, schedule.LineupEntry{Key: ct.Key, Title: t.name, OfficialRating: ct.Rating,
			Genres: ct.Genres, Year: ct.Year, RuntimeSec: int(t.runtimeMs / 1000)})
		byEntry[ct.Key] = i
	}
	// A holiday channel's genres are only its mood; what it airs in season is what fits the
	// holiday, so only that may join.
	holidays := holidayvocab.Default()
	var cands []Candidate
	for _, e := range schedule.Admitted(entries, ch.Policy, ch.Strategy) {
		if schedule.FitsHolidayChannel(e, ch.Policy, holidays) {
			cands = append(cands, lib.titles[byEntry[e.Key]].candidate)
		}
	}
	return channelView{line: line, carried: carried, need: need, candidates: cands, byKey: byEntry}
}

func loadLibrary(ctx context.Context, st FillStore) (library, error) {
	titles, err := st.LibraryTitles(ctx, time.Time{})
	if err != nil {
		return library{}, fmt.Errorf("growth: library titles: %w", err)
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.ID)
	}
	facts, err := st.LibraryItemFacts(ctx, ids)
	if err != nil {
		return library{}, fmt.Errorf("growth: library facts: %w", err)
	}
	totals, err := st.SeriesEpisodeTotals(ctx)
	if err != nil {
		return library{}, fmt.Errorf("growth: episode totals: %w", err)
	}
	lib := library{byKey: map[provision.Key]int{}}
	for _, t := range titles {
		tmdb, _ := strconv.Atoi(librarystore.IDIn(t.IDs, "tmdb"))
		tvdb, _ := strconv.Atoi(librarystore.IDIn(t.IDs, "tvdb"))
		mt, kind := provision.Movie, Film
		if t.Kind == "series" {
			mt, kind = provision.Series, Series
		}
		key, err := provision.KeyFromWebhook(mt, tmdb, tvdb)
		if err != nil {
			continue // a title with no catalog id cannot be named in a lineup
		}
		fact := facts[t.ID]
		lt := libraryTitle{name: t.Name, libraryItemID: t.ID, tmdb: tmdb, tvdb: tvdb, runtimeMs: fact.RuntimeMs,
			candidate: Candidate{Title: Title{Key: key, Kind: kind, Year: fact.Year, Rating: rating.Normalize(t.Rating),
				Genres: fact.Genres, Airable: true}}}
		if kind == Series {
			total := totals[t.ID]
			if total.Episodes == 0 {
				continue // nothing of it plays
			}
			lt.candidate.Episodes, lt.runtimeMs = total.Episodes, total.RuntimeMs
		}
		i := len(lib.titles)
		lib.titles = append(lib.titles, lt)
		// A lineup names a series by TVDB id when it has one, else TMDB; either form finds it.
		lib.byKey[key] = i
		if kind == Series && tmdb > 0 {
			lib.byKey[provision.Key("series:tmdb:"+strconv.Itoa(tmdb))] = i
		}
	}
	return lib, nil
}

// judgedAgainst is the model's verdict on each title, counting only verdicts given against line:
// one given against an older line is asked again, and until then the title has none.
func judgedAgainst(line Line, reviews map[provision.Key]channelstore.GrowthReview) func(provision.Key) Judgement {
	words := line.Words()
	return func(k provision.Key) Judgement {
		if r, ok := reviews[k]; ok && r.Line == words {
			return Judgement(r.Verdict)
		}
		return ""
	}
}

func kindOfKey(k provision.Key) Kind {
	if mt, _, _, ok := provision.ParseKey(k); ok && mt == provision.Series {
		return Series
	}
	return Film
}
