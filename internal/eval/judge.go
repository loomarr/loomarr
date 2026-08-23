//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

// judge scores a proposal's SUBJECTIVE quality (0..1) against a case's rubric using
// a second LLM call — the question the deterministic checks can't answer ("is this
// actually a good 90s action lineup?"). It is deliberately separate from the
// suggester's own model: pass any llm.Provider (the same one is fine for a first
// pass; a stronger judge model is better). Returns (score, note). A judge error is
// non-fatal — it returns (-1, reason) and the case is scored on the hard gates only.
type judgeScores struct {
	Overall     float64
	Relevance   float64
	Serendipity float64
	Reason      string
}

func judge(ctx context.Context, j llm.Provider, c Case, prop suggest.Proposal) judgeScores {
	if j == nil || c.JudgeRubric == "" {
		return judgeScores{Overall: -1, Relevance: -1, Serendipity: -1, Reason: "judge skipped (no rubric or no judge model)"}
	}
	titles := titleList(prop)
	if titles == "" {
		return judgeScores{Overall: -1, Relevance: -1, Serendipity: -1, Reason: "judge skipped (empty proposal)"}
	}
	prompt := fmt.Sprintf(`You are grading a generated TV-channel lineup for how well it fits a request.

REQUEST: %s

RUBRIC (what a good result looks like): %s

THE LINEUP THAT WAS GENERATED:
%s

Score three dimensions from 0.0 to 1.0:
- relevance: accuracy to the request and every qualifier.
- serendipity: coherent, defensible discoveries beyond only the most obvious answers. Random or off-theme picks score LOW, not high.
- overall: the quality of the lineup considering both, with relevance more important.
Reply with ONLY this JSON: {"overall": <0..1>, "relevance": <0..1>, "serendipity": <0..1>, "reason": "<one sentence>"}`,
		c.Intent.Description, c.JudgeRubric, titles)

	resp, err := j.Chat(ctx, []llm.Message{
		{Role: llm.System, Content: "You are a precise, terse evaluation judge. You output only the requested JSON."},
		{Role: llm.User, Content: prompt},
	}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		return judgeScores{Overall: -1, Relevance: -1, Serendipity: -1, Reason: fmt.Sprintf("judge call failed: %v", err)}
	}
	scores, perr := parseJudge(resp.Content)
	if perr != nil {
		return judgeScores{Overall: -1, Relevance: -1, Serendipity: -1, Reason: fmt.Sprintf("judge output unparseable: %v (%q)", perr, truncate(resp.Content, 120))}
	}
	return scores
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
func parseJudge(content string) (judgeScores, error) {
	span := extractObject(content)
	var out struct {
		Overall     float64 `json:"overall"`
		Relevance   float64 `json:"relevance"`
		Serendipity float64 `json:"serendipity"`
		Reason      string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(span), &out); err != nil {
		return judgeScores{}, err
	}
	return judgeScores{
		Overall: clampScore(out.Overall), Relevance: clampScore(out.Relevance),
		Serendipity: clampScore(out.Serendipity), Reason: out.Reason,
	}, nil
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
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
