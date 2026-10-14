//go:build eval

package eval

import (
	"context"
	"math"

	"github.com/loomarr/loomarr/internal/llm"
)

type OllamaResourceProbe struct{ client *llm.Ollama }

func NewOllamaResourceProbe(baseURL string) OllamaResourceProbe {
	return OllamaResourceProbe{client: llm.NewOllama(baseURL, "")}
}

// Measure reads Ollama's live resident-model inventory after a certification
// run. size_vram is reported separately; system RAM is the remaining resident
// bytes rather than double-counting the VRAM portion of total size.
func (p OllamaResourceProbe) Measure(ctx context.Context, identity ModelIdentity) ResourceMeasurement {
	const source = "ollama:/api/ps"
	models, err := p.client.ListResident(ctx)
	if err != nil {
		return ResourceMeasurement{Status: "unavailable", Source: source}
	}
	for _, model := range models {
		if model.Name != identity.Model {
			continue
		}
		total := int64(math.Round(model.SizeGiB * (1 << 30)))
		vram := int64(math.Round(model.VRAMGiB * (1 << 30)))
		ram := total - vram
		if ram < 0 {
			ram = 0
		}
		return ResourceMeasurement{
			Status: "measured", Source: source, PeakRAMBytes: ram, PeakVRAMBytes: vram,
		}
	}
	return ResourceMeasurement{Status: "unavailable", Source: source}
}
