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

// DefectJudgeRoute is the vision model the judge would use now: its provider and model, or
// ErrNoDefectJudge.
type DefectJudgeRoute func() (provider, model string, err error)

// DefectJudgeLedger reserves each look against the spend ceilings before the call and settles it
// with what the call cost (#551).
type DefectJudgeLedger interface {
	// ReserveDefectJudge returns held when a ceiling would be passed; nothing is spent then.
	ReserveDefectJudge(ctx context.Context, clipHash, provider, model string, at time.Time) (id string, held bool, err error)
	SettleDefectJudge(ctx context.Context, id string, attribution llm.Attribution, failed bool, at time.Time) error
}

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
	_, model, err := v.route()
	return model, err
}

// Rule implements DefectJudge. The stills are taken before the reservation, so a clip whose frames
// cannot be read spends nothing.
func (v *VisionDefectJudge) Rule(ctx context.Context, req DefectJudgeRequest) (defects.Judgement, error) {
	provider, model, err := v.route()
	if err != nil {
		return defects.Judgement{}, err
	}
	frames, err := v.tools.VisionKeyframesIn(ctx, req.File, 0, req.DurationMs, defects.JudgeStills)
	if err != nil {
		return defects.Judgement{}, fmt.Errorf("judge stills %s: %w", req.ClipHash, err)
	}
	if len(frames) == 0 {
		return defects.Judgement{}, fmt.Errorf("judge stills %s: no frames", req.ClipHash)
	}
	id, held, err := v.ledger.ReserveDefectJudge(ctx, req.ClipHash, provider, model, v.now())
	if err != nil {
		return defects.Judgement{}, err
	}
	if held {
		return defects.Judgement{}, ErrDefectJudgeHeld
	}
	resp, askErr := v.provider.AskAboutImages(llm.WithCallSite(ctx, "filler.defects.judge"),
		defects.JudgePrompt(req.Assessment, req.Era), frames)
	// Settled even when the call failed, so a failed look releases its reservation. A failed
	// settle leaves the reservation counted: the ceiling errs toward spending less.
	if err := v.ledger.SettleDefectJudge(ctx, id, resp.Attribution, askErr != nil, v.now()); err != nil && v.log != nil {
		v.log.Warn("defects: judge spend not settled", "clip", req.ClipHash, "err", err)
	}
	if askErr != nil {
		return defects.Judgement{}, askErr
	}
	// The requested model, not the resolved one: an abstention is asked again only when Model()
	// changes, and a router that resolves an alias would otherwise look like a new judge each pass.
	return defects.ParseJudgement(llm.ExtractJSONObject(resp.Content), model), nil
}
