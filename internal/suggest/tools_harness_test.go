package suggest

import (
	"context"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
)

// The helpers below drive one tool call through the same two live steps the
// suggester's tool loop uses (prepareToolCall, then executePreparedTool), with the
// loop's budget reservations but without its source initialization. They live in a
// test file because only tests call a single tool in isolation.

// runTool executes one model tool call on a fresh budget and returns the JSON
// result, the surfaced candidates, the trace and whether the call was valid.
func (s *Suggester) runTool(ctx context.Context, tc llm.ToolCall, intent Intent, feedback []FeedbackSignal) (string, []catalog.Candidate, DecisionTrace, bool) {
	ledger := newWorkLedger()
	ledger.beginGeneration()
	if !ledger.reserve(1) {
		return `{"error":"suggestion budget exhausted"}`, nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: FailureBudgetExhausted}, false
	}
	result, candidates, trace, valid, _ := s.runToolWithDateMeaning(ctx, tc, intent, feedback, nil, ledger)
	return result, candidates, trace, valid
}

// runToolWithDateMeaning is runTool against an earlier accepted date meaning and a
// caller-owned budget. It also returns the meaning this call was accepted with.
func (s *Suggester) runToolWithDateMeaning(ctx context.Context, tc llm.ToolCall, intent Intent, feedback []FeedbackSignal, accepted *ValidatedDateMeaning, ledger *workLedger) (string, []catalog.Candidate, DecisionTrace, bool, *ValidatedDateMeaning) {
	prepared, result, trace, valid := prepareToolCall(tc, intent, accepted)
	if result != "" || !valid || prepared.meaning.DateMeaning().Kind == DateMeaningAmbiguous {
		return result, nil, trace, valid, func() *ValidatedDateMeaning {
			if valid {
				return &prepared.meaning
			}
			return nil
		}()
	}
	extra := 0
	if prepared.discoveryMode {
		extra = len(prepared.queries) - 1
	}
	if !ledger.reserve(extra) {
		return `{"error":"suggestion budget exhausted"}`, nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: FailureBudgetExhausted}, true, &prepared.meaning
	}
	result, candidates, trace, valid := s.executePreparedTool(ctx, prepared, intent, feedback)
	return result, candidates, trace, valid, &prepared.meaning
}

// parseDiscoveryQuery parses discovery arguments with no accepted date meaning, so
// a date-only discovery is refused.
func parseDiscoveryQuery(args map[string]any) (catalog.DiscoveryQuery, bool, error) {
	return parseDiscoveryQueryWithDateQualifier(args, false)
}
