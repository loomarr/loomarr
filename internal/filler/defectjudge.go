package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/llm"
)

// ErrNoDefectJudge means no vision model is configured to rule on borderline clips. They fail with
// a reason that says so, and are ruled on once one is.
var ErrNoDefectJudge = errors.New("no vision model is configured")

// ErrDefectJudgeHeld means the judge's spend ceiling is reached. The clip stays pending and the
// next pass asks again, as it does after a provider outage (#548).
var ErrDefectJudgeHeld = errors.New("defect judge spend ceiling reached")

// DefectJudgeRequest is one borderline clip put to the judge.
type DefectJudgeRequest struct {
	ClipHash   string
	File       string
	DurationMs int64
	Era        int
	Assessment defects.Assessment
}

// DefectJudge rules on clips in the defect floor's borderline band (#2034).
type DefectJudge interface {
	// Model names the judge that would rule now, or returns ErrNoDefectJudge.
	Model(ctx context.Context) (string, error)
	// Rule returns a ruling, or an error when none could be had: ErrNoDefectJudge, ErrDefectJudgeHeld,
	// or a provider failure. Every error leaves the clip to be asked again.
	Rule(ctx context.Context, req DefectJudgeRequest) (defects.Judgement, error)
}

// DefectJudgeEndpoint is the vision model the judge would ask now.
type DefectJudgeEndpoint struct {
	Provider, Model string
	// SelfHosted is the endpoint's identity, not the provider's name: an operator-run server is
	// free whatever it is called.
	SelfHosted bool
}

// Name identifies the judge in a ruling: an abstention is asked again only of another name.
func (e DefectJudgeEndpoint) Name() string { return e.Provider + "/" + e.Model }

// DefectJudgeRoute resolves the judge's endpoint now, or returns ErrNoDefectJudge.
type DefectJudgeRoute func() (DefectJudgeEndpoint, error)

// DefectJudgeLedger reserves each look against the spend ceilings before the call and settles it
// with what the call cost (#551).
type DefectJudgeLedger interface {
	// ReserveDefectJudge returns held when a ceiling would be passed; nothing is spent then.
	ReserveDefectJudge(ctx context.Context, clipHash string, e DefectJudgeEndpoint, at time.Time) (id string, held bool, err error)
	SettleDefectJudge(ctx context.Context, id string, e DefectJudgeEndpoint, attribution llm.Attribution, failed bool, at time.Time) error
}

// ErrDefectJudgeClip marks a failure that belongs to the clip rather than the judge: stills that
// could not be read, or a reply that could not be read. Other clips are still asked this pass.
var ErrDefectJudgeClip = errors.New("the judge could not rule on this clip")

// VisionDefectJudge shows a vision model stills from the clip with what the detectors found.
type VisionDefectJudge struct {
	provider llm.VisionProvider
	tools    MediaTools
	ledger   DefectJudgeLedger
	route    DefectJudgeRoute
	now      func() time.Time
	log      *slog.Logger
}

// NewVisionDefectJudge builds the defect floor's judge.
func NewVisionDefectJudge(provider llm.VisionProvider, tools MediaTools, ledger DefectJudgeLedger,
	route DefectJudgeRoute, now func() time.Time, log *slog.Logger) *VisionDefectJudge {
	return &VisionDefectJudge{provider: provider, tools: tools, ledger: ledger, route: route, now: now, log: log}
}

// Model implements DefectJudge.
func (v *VisionDefectJudge) Model(context.Context) (string, error) {
	e, err := v.route()
	return e.Name(), err
}

// Rule implements DefectJudge. The stills are taken before the reservation, so a clip whose frames
// cannot be read spends nothing.
func (v *VisionDefectJudge) Rule(ctx context.Context, req DefectJudgeRequest) (defects.Judgement, error) {
	e, err := v.route()
	if err != nil {
		return defects.Judgement{}, err
	}
	frames, err := v.tools.VisionKeyframesIn(ctx, req.File, 0, req.DurationMs, defects.JudgeStills)
	if err == nil && len(frames) == 0 {
		err = errors.New("no frames")
	}
	if err != nil {
		return defects.Judgement{}, fmt.Errorf("%w: stills: %w", ErrDefectJudgeClip, err)
	}
	id, held, err := v.ledger.ReserveDefectJudge(ctx, req.ClipHash, e, v.now())
	if err != nil {
		return defects.Judgement{}, err
	}
	if held {
		return defects.Judgement{}, ErrDefectJudgeHeld
	}
	resp, askErr := v.provider.AskAboutImages(llm.WithCallSite(ctx, "filler.defects.judge"),
		defects.JudgePrompt(req.Assessment, req.Era), frames)
	// Settled even when the call failed or the pass is being cancelled: an unsettled look stays
	// counted at its reservation and its answer would be thrown away and bought again.
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := v.ledger.SettleDefectJudge(settleCtx, id, e, resp.Attribution, askErr != nil, v.now()); err != nil && v.log != nil {
		v.log.Warn("defects: judge spend not settled", "clip", req.ClipHash, "err", err)
	}
	if askErr != nil {
		return defects.Judgement{}, askErr
	}
	j, err := defects.ParseJudgement(llm.ExtractJSONObject(resp.Content), e.Name())
	if err != nil {
		return defects.Judgement{}, fmt.Errorf("%w: %w", ErrDefectJudgeClip, err)
	}
	return j, nil
}
