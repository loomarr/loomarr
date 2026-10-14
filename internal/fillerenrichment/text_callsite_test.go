package fillerenrichment

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// siteProvider records the call site each Chat arrived with; the OpenAI client logs and
// counts by that name, so an unnamed call shows up as "unknown" in production.
type siteProvider struct{ sites []string }

func (p *siteProvider) Name() string { return "fixture" }

func (p *siteProvider) Chat(ctx context.Context, _ []llm.Message, _ llm.ChatOptions) (llm.Response, error) {
	p.sites = append(p.sites, llm.CallSite(ctx))
	return llm.Response{Content: `{}`}, nil
}

func TestClassifyTextBatch_NamesItsLLMCallSite(t *testing.T) {
	provider := &siteProvider{}
	forest := taxonomy.New([]taxonomy.Taxon{{Slug: "commercial", Label: "Commercial", Axis: taxonomy.AxisFormat}})
	signals := Signals{ClipHash: "c", Title: "T", ObservedAt: time.Unix(500, 0).UTC()}
	// The reply content is irrelevant: the site is fixed before the response is parsed.
	_, _, _ = classifyTextBatch(t.Context(), provider, forest, []textBatchInput{{Axes: textAxes, Signals: signals}}, "m", "p", "t")
	if len(provider.sites) != 1 || provider.sites[0] != "filler.text_batch" {
		t.Fatalf("call sites = %v, want [filler.text_batch]", provider.sites)
	}
}
