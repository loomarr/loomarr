package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// ReviewsPerRun is how many titles one run asks the model about: the job runs often and asks
// little each time, so it never holds the AI server for long (decision 33).
const ReviewsPerRun = 4

// seedsInPrompt is how many of the channel's own titles the question names as examples.
const seedsInPrompt = 8

// ReviewStore is the slice of the store the review reads and writes.
type ReviewStore interface {
	FillStore
	SaveGrowthReview(ctx context.Context, r channelstore.GrowthReview) error
}

// Editor is the operator write path a rejected title is taken off by (edit.Service). Taking a
// grown title off declines it, so the nightly fill leaves it out.
type Editor interface {
	Edit(ctx context.Context, backend edit.Backend, id string, revision int64, change func(*channelstore.Channel) error) (channelstore.Channel, error)
}

// Model is the language model the review asks, and the name its verdicts are kept under.
type Model interface {
	llm.Provider
	Model() string
}

// Spend meters questions against the day's AI spending cap, the one cap filler work also draws on
// (#3838 decision 19). ReserveReview holds one question's price before it is asked; ok is false at
// the cap, and the question waits for another day while the code's choices stand. SettleReview
// then records what the call cost, failed or not. A self-hosted model costs nothing and is never
// held; its id is empty and settling it does nothing.
type Spend interface {
	ReserveReview(ctx context.Context, channelID string, key provision.Key, at time.Time) (id string, ok bool, err error)
	SettleReview(ctx context.Context, id string, cost llm.Attribution, failed bool, at time.Time) error
}

// Review is the `channel-growth-review` job (#4333, decisions 8, 32 and 33): in the background, a
// few titles at a time, the model judges the titles growth added to a channel and the unclear fits
// it held back. A title growth added that the model judges does not fit is taken off the channel;
// an unclear fit it judges fits joins at the next fill. A title a person put on by hand is a seed
// and is never reviewed, so putting a rejected title back is the undo.
type Review struct {
	store  ReviewStore
	model  Model
	editor Editor
	spend  Spend
	busy   func() bool // a person is waiting on the model
	now    func() time.Time
	log    *slog.Logger
}

// NewReview wires the job. busy reports interactive model work in flight; the review waits for it.
func NewReview(st ReviewStore, model Model, ed Editor, spend Spend, busy func() bool, now func() time.Time, log *slog.Logger) *Review {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	if busy == nil {
		busy = func() bool { return false }
	}
	return &Review{store: st, model: model, editor: ed, spend: spend, busy: busy, now: now, log: log}
}

// Job returns the review as a scheduler job.
func (r *Review) Job() jobs.Job {
	return jobs.Job{
		Name: "channel-growth-review", Group: jobs.GroupChannels, Title: "Review what channels grew by",
		Description: "Asks the model whether the titles a channel took in by itself belong on it.",
		DefaultCron: "0 */5 * * * *", ScheduleKey: settings.JobGrowthReviewSchedule.Key(),
		Timeout: 5 * time.Minute,
		Run:     func(ctx context.Context) error { _, err := r.Run(ctx); return err },
	}
}

// question is one title to ask about.
type question struct {
	ch    channelstore.Channel
	line  Line
	title libraryTitle
	grown bool // on the lineup by growth: a "does not fit" takes it off
}

// Run asks about up to ReviewsPerRun titles and reports how many it judged.
func (r *Review) Run(ctx context.Context) (int, error) {
	if r.model == nil || r.model.Model() == "" || r.busy() {
		return 0, nil
	}
	lib, err := loadLibrary(ctx, r.store)
	if err != nil {
		return 0, err
	}
	queue, err := r.queue(ctx, lib)
	if err != nil {
		return 0, err
	}
	judged := 0
	for _, q := range queue {
		if r.busy() {
			break // a person came along: they get the model first
		}
		spendID, ok, err := r.spend.ReserveReview(ctx, q.ch.ID, q.title.candidate.Title.Key, r.now())
		if err != nil {
			return judged, err
		}
		if !ok {
			r.log.Info("growth review: the day's AI spending cap is reached; reviews wait")
			break
		}
		verdict, reason, cost, err := r.ask(ctx, q)
		if serr := r.spend.SettleReview(ctx, spendID, cost, err != nil, r.now()); serr != nil {
			return judged, serr
		}
		if err != nil {
			// An answer outside the fixed choices, or a model that cannot be reached, leaves the
			// title unjudged; it is asked again on a later run.
			r.log.Warn("growth review: no verdict", "channel", q.ch.ID, "title", q.title.candidate.Title.Key, "err", err)
			continue
		}
		if err := r.store.SaveGrowthReview(ctx, channelstore.GrowthReview{
			ChannelID: q.ch.ID, Key: q.title.candidate.Title.Key, Line: q.line.Words(), Verdict: string(verdict),
			Reason: reason, Model: r.model.Model(), ReviewedAt: r.now(),
		}); err != nil {
			return judged, err
		}
		judged++
		if verdict == JudgedNotFit && q.grown {
			if err := r.takeOff(ctx, q.ch.ID, q.title.candidate.Title.Key); err != nil {
				return judged, err
			}
			r.log.Info("growth review: a grown title does not fit and is taken off", "channel", q.ch.ID,
				"title", q.title.candidate.Title.Key, "reason", reason)
		}
	}
	return judged, nil
}

// queue is what to ask about this run: first the titles growth already added, which are airing,
// then the unclear fits that would join a short channel if the model accepted them.
func (r *Review) queue(ctx context.Context, lib library) ([]question, error) {
	chans, err := r.store.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("growth review: list channels: %w", err)
	}
	var grown, unclear []question
	for _, ch := range chans {
		if !ch.Status.Reconcilable() || !ch.Policy.Grows() || !ch.FromRequest {
			continue
		}
		v := view(ch, lib)
		if v.line.Fixed {
			continue
		}
		reviews, err := r.store.GrowthReviews(ctx, ch.ID)
		if err != nil {
			return nil, err
		}
		judged := judgedAgainst(v.line, reviews)
		for _, e := range ch.Lineup {
			if i, held := lib.byKey[e.Key]; held && e.Grown != "" && judged(e.Key) == "" {
				grown = append(grown, question{ch: ch, line: v.line, title: lib.titles[i], grown: true})
			}
		}
		// The unclear fits that matter are the ones the plan would take if each were accepted.
		for _, p := range Plan(v.line, v.candidates, v.carried, TargetFor(v.need, MaxShowShare), Bar, ch.Policy.Declined, judged, true) {
			if p.Fit == Unclear && judged(p.Key) == "" {
				unclear = append(unclear, question{ch: ch, line: v.line, title: lib.titles[v.byKey[p.Key]]})
			}
		}
	}
	queue := append(grown, unclear...)
	if len(queue) > ReviewsPerRun {
		queue = queue[:ReviewsPerRun]
	}
	return queue, nil
}

// answer is the model's reply: one of two words and a few of its own.
type answer struct {
	Answer string `json:"answer"`
	Reason string `json:"reason"`
}

// ask puts the question to the model. cost is what the call reported, even when the reply is
// unusable: a provider bills a reply that was not the asked JSON all the same.
func (r *Review) ask(ctx context.Context, q question) (j Judgement, reason string, cost llm.Attribution, err error) {
	t := q.title.candidate.Title
	kind := "film"
	if t.Kind == Series {
		kind = "series"
	}
	var seeds []string
	for _, e := range q.ch.Lineup {
		if e.Grown == "" && e.Title != "" && len(seeds) < seedsInPrompt {
			seeds = append(seeds, e.Title)
		}
	}
	about := fmt.Sprintf("a %s from %d", kind, t.Year)
	if t.Year == 0 {
		about = "a " + kind
	}
	if t.Rating != "" {
		about += ", rated " + string(t.Rating)
	}
	if len(t.Genres) > 0 {
		about += ", " + strings.Join(t.Genres, ", ")
	}
	prompt := fmt.Sprintf("A TV channel called %q airs %s. It already airs: %s.\n\n"+
		"Does %q (%s) belong on this channel?\n\n"+
		`Reply with JSON only: {"answer": "fits" or "does_not_fit", "reason": "at most 12 words"}`,
		q.ch.Name, q.line.Words(), strings.Join(seeds, "; "), q.title.name, about)
	zero := 0.0
	resp, err := r.model.Chat(ctx, []llm.Message{
		{Role: llm.System, Content: "You decide whether a title belongs on a TV channel. Answer with JSON only."},
		{Role: llm.User, Content: prompt},
	}, llm.ChatOptions{JSONMode: true, Temperature: &zero, MaxTokens: 120})
	cost = resp.Attribution
	if err != nil {
		return "", "", cost, err
	}
	var a answer
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(resp.Content)), &a); err != nil {
		return "", "", cost, fmt.Errorf("reply is not the asked JSON: %w", err)
	}
	j = Judgement(strings.TrimSpace(strings.ToLower(a.Answer)))
	if j != JudgedFits && j != JudgedNotFit {
		return "", "", cost, fmt.Errorf("answer %q is not one of the choices", a.Answer)
	}
	reason = strings.TrimSpace(a.Reason)
	if words := strings.Fields(reason); len(words) > 20 {
		reason = strings.Join(words[:20], " ")
	}
	return j, reason, cost, nil
}

// takeOff removes a grown title from the channel through the operator write path, which declines
// it. A title already gone (a person took it off first) is nothing to do.
func (r *Review) takeOff(ctx context.Context, channelID string, key provision.Key) error {
	_, err := r.editor.Edit(ctx, edit.Backend{}, channelID, 0, func(ch *channelstore.Channel) error {
		i := slices.IndexFunc(ch.Lineup, func(e schedule.LineupEntry) bool { return e.Key == key && e.Grown != "" })
		if i < 0 {
			return errNothingToTakeOff
		}
		ch.Lineup = slices.Delete(slices.Clone(ch.Lineup), i, i+1)
		return nil
	})
	if errors.Is(err, errNothingToTakeOff) {
		return nil
	}
	return err
}

var errNothingToTakeOff = errors.New("growth review: the title is no longer a grown entry")
