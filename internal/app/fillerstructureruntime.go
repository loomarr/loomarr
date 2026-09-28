package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
	"github.com/loomarr/loomarr/internal/fillerstructurewindowopenrouter"
	"github.com/loomarr/loomarr/internal/store"
)

type productionStructureWindowLedger struct {
	store  store.FillerStructureAssessmentStore
	budget store.InferenceBudget
}

func (l productionStructureWindowLedger) Reserve(ctx context.Context, reservation fillerstructurewindow.CallReservation) (fillerstructurewindow.CallReservationState, error) {
	return l.store.ReserveStructureWindowCall(ctx, reservation, l.budget)
}

func (l productionStructureWindowLedger) Settle(ctx context.Context, record fillerstructurewindow.CallRecord) error {
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

func buildCertifiedWindowStructureRuntime(st store.Store, set resolved, layout filler.Layout,
	authority *fillerstructurewindow.MaterializationAuthority,
	deployment *fillerstructurewindowopenrouter.Deployment,
) (filler.CompleteTimelineStructureDecisioner, error) {
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
	return fillerstructurewindowopenrouter.NewCertifiedRuntime(fillerstructurewindowopenrouter.CertifiedRuntimeConfig{
		Authority: *authority, Deployment: *deployment, APIKey: apiKey,
		SourceRoot: layout.ClipDir(), MediaRoot: filepath.Join(root, "media"), EvidenceRoot: filepath.Join(root, "evidence"),
		FFmpegPath: resolveTool(set.str("playout.ffmpeg_path"), "ffmpeg"),
		Ledger: productionStructureWindowLedger{store: st, budget: store.InferenceBudget{
			PerClipNanoUSD: deployment.PerSourceBudgetNanoUSD, PerDayNanoUSD: deployment.PerDayBudgetNanoUSD,
		}},
	})
}

func openRouterStructureAPIKey(set resolved) string {
	if value, err := set.svc.LoadRaw(setLLMAPIKey + ".openrouter"); err == nil && value != "" {
		return value
	}
	selection := resolveSelection(set)
	if selection.Provider == "openrouter" {
		return selection.APIKey
	}
	return ""
}
