package routine

import (
	"context"
	"errors"
	"log/slog"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/research"
)

// EnrichmentRunner is the descriptive-enrichment pass the driver runs after preparation.
type EnrichmentRunner interface {
	Run(context.Context) (enrichment.RunResult, error)
}

// PipelineDriver owns one operator-facing background loop while preserving two independent
// authority boundaries inside it: readiness prepares clips for playback; details enrich clips
// that are already ready. A preparation failure does not strand the ready backlog, but a cancelled
// lease stops before beginning more work.
type PipelineDriver struct {
	prepare func(context.Context) (filler.PipelineResult, error)
	details func(context.Context) error
	// deferred counts consecutive passes that skipped details because preparation was advancing.
	deferred int
}

// detailMaxDeferrals bounds how many consecutive advancing passes may postpone details.
// ⚠ Deferral used to key on "anything is Runnable or InProgress", which is a property of the
// backlog, not of progress: a backlog that never drains (production held 202 clips, 34 ready)
// starved enrichment forever and filler_enrichment_passes stayed empty (#1412).
const detailMaxDeferrals = 3

// NewDriver returns a driver over a preparation pass and a details pass; either may be nil.
func NewDriver(prepare func(context.Context) (filler.PipelineResult, error), details func(context.Context) error) *PipelineDriver {
	return &PipelineDriver{prepare: prepare, details: details}
}

// NewPipelineDriver returns the driver over the ingest pipeline and the detail runner. Either may
// be nil, and log may be nil.
func NewPipelineDriver(pipeline *filler.Pipeline, details EnrichmentRunner, log *slog.Logger) *PipelineDriver {
	return &PipelineDriver{
		prepare: func(ctx context.Context) (filler.PipelineResult, error) {
			if pipeline == nil {
				return filler.PipelineResult{}, nil
			}
			return pipeline.RunOnce(ctx)
		},
		details: func(ctx context.Context) error {
			if details == nil {
				return nil
			}
			result, err := details.Run(ctx)
			if log != nil {
				log.Info("filler details run", "considered", result.Considered,
					"updated", result.Updated, "failed", result.Failed)
			}
			return err
		},
	}
}

func (d *PipelineDriver) Run(ctx context.Context) error {
	var prepared filler.PipelineResult
	var prepareErr error
	if d.prepare != nil {
		prepared, prepareErr = d.prepare(ctx)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(prepareErr, err)
	}
	// Descriptive enrichment is optional and may involve a remote model. While preparation is
	// actually advancing clips, let the next bounded pass keep moving them toward playable
	// readiness — but only for a bounded number of passes, and never on the strength of a backlog
	// that did not advance. Scheduled retries do not block enrichment because they cannot advance.
	preparing := prepared.Advanced > 0 && (prepared.Overview.Runnable > 0 || prepared.Overview.InProgress > 0)
	if preparing && d.deferred < detailMaxDeferrals {
		d.deferred++
		return prepareErr
	}
	d.deferred = 0
	var detailsErr error
	if d.details != nil {
		detailsErr = d.details(ctx)
	}
	return errors.Join(prepareErr, detailsErr)
}

// DetailRunner keeps one scheduler row while the modules retain different authority:
// enrichment writes verified axes; research writes only cited suggestions.
type DetailRunner struct {
	enrichment EnrichmentRunner
	research   *research.Runner
	retire     *GeographyRetirement // runs last, once geography has had every pass
}

// NewDetailRunner returns the detail pass: enrichment, then research, then geography retirement.
// Each may be nil.
func NewDetailRunner(enrichment EnrichmentRunner, research *research.Runner, retire *GeographyRetirement) DetailRunner {
	return DetailRunner{enrichment: enrichment, research: research, retire: retire}
}

func (r DetailRunner) Run(ctx context.Context) (enrichment.RunResult, error) {
	var out enrichment.RunResult
	var enrichmentErr, researchErr, retireErr error
	if r.enrichment != nil {
		out, enrichmentErr = r.enrichment.Run(ctx)
	}
	if r.research != nil {
		var researched research.RunResult
		researched, researchErr = r.research.Run(ctx)
		out.Considered += researched.Considered
		out.Updated += researched.Updated
		out.Failed += researched.Failed
	}
	if r.retire != nil {
		_, retireErr = r.retire.Run(ctx)
	}
	return out, errors.Join(enrichmentErr, researchErr, retireErr)
}
