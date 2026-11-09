package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/filler/structure/window/openrouter"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

type productionStructureWindowLedger struct {
	store  records.FillerStructureAssessmentStore
	budget records.InferenceBudget
}

func (l productionStructureWindowLedger) Reserve(ctx context.Context, reservation window.CallReservation) (window.CallReservationState, error) {
	return l.store.ReserveStructureWindowCall(ctx, reservation, l.budget)
}

func (l productionStructureWindowLedger) Settle(ctx context.Context, record window.CallRecord) error {
	return l.store.SettleStructureWindowCall(ctx, record)
}

// liveStructureRuntime serves the long-reel runtime for the two long-reel files currently set
// (#1659: they apply live). A path edit builds a complete new runtime (authority, deployment,
// assessment, gate and shadow) and replaces the old one in one assignment, so a reader gets one
// or the other and never a mix.
//
// ⚠ Keyed on the PATHS, not the file contents. Both files are reviewed, hash-pinned evidence; a
// new review is a new file. A failed build is cached like a good one, so a bad path logs once
// rather than on every split, and every proposal stays held until the path changes.
type liveStructureRuntime struct {
	paths func() (authority, deployment string)
	build func(authority, deployment string) filler.StructureRuntime

	mu                  sync.Mutex
	built               bool
	authority, deployed string
	current             filler.StructureRuntime
}

func (l *liveStructureRuntime) Current() filler.StructureRuntime {
	authority, deployment := l.paths()
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.built || authority != l.authority || deployment != l.deployed {
		l.current = l.build(authority, deployment)
		l.built, l.authority, l.deployed = true, authority, deployment
	}
	return l.current
}

func buildCertifiedWindowStructureRuntime(st records.Store, set resolved, layout cliplibrary.Layout,
	authority *window.MaterializationAuthority,
	deployment *openrouter.Deployment, cutFamily func() encode.Family,
) (split.CompleteTimelineDecisioner, error) {
	if authority == nil || deployment == nil {
		return nil, nil
	}
	if layout.ClipDir() == "" {
		return nil, errors.New("certified long-reel runtime requires filler storage")
	}
	apiKey := openRouterStructureAPIKey(set)
	if apiKey == "" {
		return nil, errors.New("certified long-reel runtime requires the OpenRouter provider secret")
	}
	root := filepath.Join(layout.ClipDir(), ".loomarr", "structure-window")
	return openrouter.NewCertifiedRuntime(openrouter.CertifiedRuntimeConfig{
		Authority: *authority, Deployment: *deployment, APIKey: apiKey,
		SourceRoot: layout.ClipDir(), MediaRoot: filepath.Join(root, "media"), EvidenceRoot: filepath.Join(root, "evidence"),
		FFmpegPath: resolveTool(settings.PlayoutFFmpegPath.Get(set), "ffmpeg"), CutFamily: cutFamily,
		Ledger: productionStructureWindowLedger{store: st, budget: records.InferenceBudget{
			PerClipNanoUSD: deployment.PerSourceBudgetNanoUSD, PerDayNanoUSD: deployment.PerDayBudgetNanoUSD,
		}},
	})
}

func openRouterStructureAPIKey(set resolved) string {
	if value, err := set.svc.LoadRaw(settings.LLMAPIKey.Key() + ".openrouter"); err == nil && value != "" {
		return value
	}
	selection := resolveSelection(set)
	if selection.Provider == "openrouter" {
		return selection.APIKey
	}
	return ""
}
