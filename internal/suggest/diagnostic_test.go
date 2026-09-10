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
		Content:     `{"channelName":"Signal Cinema","rationale":"Grounded science fiction.","picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix","rationale":"A grounded science-fiction match.","confidence":0.98}],"policy":{"genres":{"include":["Science Fiction"]}},"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`,
		Attribution: llm.Attribution{RequestedProvider: "diagnostic", RequestedModel: "fixture-model", Charge: &llm.Money{Amount: "0.001", Currency: "USD"}},
	}, nil
}

func TestRunToolFinalizationDiagnosticUsesFrozenPostResultContract(t *testing.T) {
	provider := &diagnosticProvider{}
	report, err := RunToolFinalizationDiagnostic(context.Background(), provider, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if report.PromptVersion != "suggester-prompt-v9" || report.ToolSchemaVersion != "catalog-search-v6" || report.MessageTemplateVersion != "planner-tool-result-finalization-v3" {
		t.Fatalf("contract identity = %+v", report)
	}
	if len(report.SystemPromptSHA256) != 64 || len(report.UserPromptSHA256) != 64 || len(report.MessagesSHA256) != 64 || len(report.ToolSchemaSHA256) != 64 {
		t.Fatalf("diagnostic hashes = system %q user %q messages %q tool %q", report.SystemPromptSHA256, report.UserPromptSHA256, report.MessagesSHA256, report.ToolSchemaSHA256)
	}
	if report.SystemPromptSHA256 != "3818a5fd969b40a5a57510f7aa5fdb56bc134d03acb3b78b32f52ada75bf5478" ||
		report.UserPromptSHA256 != "37f435b49e8c33fa43f3452f2a6bf7c761e2012facec6d1a93378db8891d48cc" ||
		report.MessagesSHA256 != "dec08b9d0026bcd71df02ae750d8cdefdf8351e8b6cdc7fe32d98529ffc5c4ea" ||
		report.ToolSchemaSHA256 != "595a6ec8416fc9b0cf438cc86897006d69f5f2fcf6de238c0e310e8de8273f86" {
		t.Fatalf("frozen diagnostic identity drifted without a version change: %+v", report)
	}
	if got, want := report.MessageRoles, []string{"system", "user", "assistant", "tool", "user"}; !equalStrings(got, want) {
		t.Fatalf("message roles = %v, want %v", got, want)
	}
	if len(provider.messages) != 5 || provider.messages[3].ToolCallID != PlannerDiagnosticToolCallID {
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
