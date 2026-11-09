package app

import (
	"errors"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/filler/structure/window/openrouter"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

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
		Ledger: storeadapter.NewStructureWindowLedger(st, records.InferenceBudget{
			PerClipNanoUSD: deployment.PerSourceBudgetNanoUSD, PerDayNanoUSD: deployment.PerDayBudgetNanoUSD,
		}),
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
