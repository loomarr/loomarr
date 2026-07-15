//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/suggest"
)

// judge scores a proposal's SUBJECTIVE quality (0..1) against a case's rubric using
// a second LLM call — the question the deterministic checks can't answer ("is this
// actually a good 90s action lineup?"). It is deliberately separate from the
// suggester's own model: pass any llm.Provider (the same one is fine for a first
// pass; a stronger judge model is better). Returns (score, note). A judge error is
// non-fatal — it returns (-1, reason) and the case is scored on the hard gates only.
func judge(ctx context.Context, j llm.Provider, c Case, prop suggest.Proposal) (float64, string) {
	if j == nil || c.JudgeRubric == "" {
		return -1, "judge skipped (no rubric or no judge model)"
	}
	titles := titleList(prop)
	if titles == "" {
		return -1, "judge skipped (empty proposal)"
	}
	prompt := fmt.Sprintf(`You are grading a generated TV-channel lineup for how well it fits a request.

REQUEST: %s

RUBRIC (what a good result looks like): %s

THE LINEUP THAT WAS GENERATED:
%s

Score how well the lineup satisfies the request, from 0.0 (completely wrong) to 1.0 (excellent).
Reply with ONLY this JSON: {"score": <0..1>, "reason": "<one sentence>"}`,
		c.Intent.Description, c.JudgeRubric, titles)

	resp, err := j.Chat(ctx, []llm.Message{
		{Role: llm.System, Content: "You are a precise, terse evaluation judge. You output only the requested JSON."},
		{Role: llm.User, Content: prompt},
	}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		return -1, fmt.Sprintf("judge call failed: %v", err)
	}
	score, reason, perr := parseJudge(resp.Content)
	if perr != nil {
		return -1, fmt.Sprintf("judge output unparseable: %v (%q)", perr, truncate(resp.Content, 120))
	}
	return score, reason
}

// titleList renders the proposal's grounded titles for the judge to read.
func titleList(p suggest.Proposal) string {
	var b strings.Builder
	for _, it := range p.Lineup {
		fmt.Fprintf(&b, "- %s (%d) [in library]\n", it.Name, it.Year)
	}
	for _, it := range p.Acquisitions {
		fmt.Fprintf(&b, "- %s (%d) [to acquire]\n", it.Name, it.Year)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// parseJudge extracts {"score":..,"reason":..} from the judge's reply, tolerating a
// code fence or surrounding prose (same robustness the suggester needs).
func parseJudge(content string) (float64, string, error) {
	span := extractObject(content)
	var out struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(span), &out); err != nil {
		return 0, "", err
	}
	if out.Score < 0 {
		out.Score = 0
	}
	if out.Score > 1 {
		out.Score = 1
	}
	return out.Score, out.Reason, nil
}

// extractObject returns the outermost {...} span (string/escape aware) — a local
// copy of the suggester's extractJSONObject so the eval doesn't reach into an
// unexported helper.
func extractObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return s
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s
}
