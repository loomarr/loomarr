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
	if report.PromptVersion != "suggester-prompt-v3" || report.ToolSchemaVersion != "catalog-search-v3" || report.MessageTemplateVersion != "planner-tool-result-finalization-v1" {
		t.Fatalf("contract identity = %+v", report)
	}
	if len(report.SystemPromptSHA256) != 64 || len(report.UserPromptSHA256) != 64 || len(report.MessagesSHA256) != 64 || len(report.ToolSchemaSHA256) != 64 {
		t.Fatalf("diagnostic hashes = system %q user %q messages %q tool %q", report.SystemPromptSHA256, report.UserPromptSHA256, report.MessagesSHA256, report.ToolSchemaSHA256)
	}
	if report.SystemPromptSHA256 != "c825bb321636ee756635167828bd252e46c550692835e56950951b7c5269ae61" ||
		report.UserPromptSHA256 != "37f435b49e8c33fa43f3452f2a6bf7c761e2012facec6d1a93378db8891d48cc" ||
		report.MessagesSHA256 != "66e6d3d3dbd30a5f36e69a59de16b64a5080a77b68e78577cd2fb9de12ac10d9" ||
		report.ToolSchemaSHA256 != "16a9f228864ae8286df2fbe5439fe121922a35402f7168f1a42319652bf30853" {
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
