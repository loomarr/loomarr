package suggest

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

type diagnosticProvider struct {
	messages []llm.Message
	opts     llm.ChatOptions
}

func (p *diagnosticProvider) Name() string { return "diagnostic" }

func (p *diagnosticProvider) Chat(_ context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	p.messages = append([]llm.Message(nil), messages...)
	p.opts = opts
	return llm.Response{
		Content:     `{"channelName":"Signal Cinema","rationale":"Grounded science fiction.","picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix","rationale":"A grounded science-fiction match.","confidence":0.98}],"policy":{"genres":{"include":["Science Fiction"]}}}`,
		Attribution: llm.Attribution{RequestedProvider: "diagnostic", RequestedModel: "fixture-model", Charge: &llm.Money{Amount: "0.001", Currency: "USD"}},
	}, nil
}

func TestRunToolFinalizationDiagnosticUsesFrozenPostResultContract(t *testing.T) {
	provider := &diagnosticProvider{}
	report, err := RunToolFinalizationDiagnostic(context.Background(), provider, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if report.PromptVersion != "suggester-prompt-v2" || report.ToolSchemaVersion != "catalog-search-v2" || report.MessageTemplateVersion != "planner-tool-result-finalization-v1" {
		t.Fatalf("contract identity = %+v", report)
	}
	if len(report.SystemPromptSHA256) != 64 || len(report.UserPromptSHA256) != 64 || len(report.MessagesSHA256) != 64 || len(report.ToolSchemaSHA256) != 64 {
		t.Fatalf("diagnostic hashes = system %q user %q messages %q tool %q", report.SystemPromptSHA256, report.UserPromptSHA256, report.MessagesSHA256, report.ToolSchemaSHA256)
	}
	if report.SystemPromptSHA256 != "5e2e8153e17e6a09262c94dd9c956a041228b0a63808b9afe782926ffa4a81af" ||
		report.UserPromptSHA256 != "37f435b49e8c33fa43f3452f2a6bf7c761e2012facec6d1a93378db8891d48cc" ||
		report.MessagesSHA256 != "9817bf525d2d10ac7e1c97fde40e0ac58f9f1ef128c914306787b64dabb20f4a" ||
		report.ToolSchemaSHA256 != "cb8df1aa2e61f7c5a153af762dee0c5389f78fee1088f5d190df91d377f4f147" {
		t.Fatalf("frozen diagnostic identity drifted without a version change: %+v", report)
	}
	if got, want := report.MessageRoles, []string{"system", "user", "assistant", "tool"}; !equalStrings(got, want) {
		t.Fatalf("message roles = %v, want %v", got, want)
	}
	if len(provider.messages) != 4 || provider.messages[3].ToolCallID != PlannerDiagnosticToolCallID {
		t.Fatalf("provider messages = %+v", provider.messages)
	}
	if !provider.opts.JSONMode || len(provider.opts.Tools) != 0 {
		t.Fatalf("post-result options = %+v", provider.opts)
	}
	if !report.JSONValid || report.RepeatedToolCall || report.ResponseContentSHA256 == "" {
		t.Fatalf("result = %+v", report)
	}
	if report.ChargeAmount != "0.001" || report.ChargeCurrency != "USD" {
		t.Fatalf("charge = %q %q", report.ChargeAmount, report.ChargeCurrency)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
