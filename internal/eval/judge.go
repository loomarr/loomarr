//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

// judge scores a proposal's SUBJECTIVE quality (0..1) against a case's rubric using
// a second LLM call — the question the deterministic checks can't answer ("is this
// actually a good 90s action lineup?"). It is deliberately separate from the
// suggester's own model: pass any llm.Provider (the same one is fine for a first
// pass; a stronger judge model is better). Judge-stage failures are ordinary
// errors so they cannot be confused with a valid low score.
// JudgeScores is one successful subjective assessment in the closed 0..1 range.
type JudgeScores struct {
	Overall     float64
	Relevance   float64
	Serendipity float64
	Reason      string
}

type modelJudge struct{ provider llm.Provider }

func (j modelJudge) Score(ctx context.Context, c Case, prop suggest.Proposal) (JudgeScores, error) {
	return judge(ctx, j.provider, c, prop)
}

func judge(ctx context.Context, j llm.Provider, c Case, prop suggest.Proposal) (JudgeScores, error) {
	if j == nil {
		return JudgeScores{}, errors.New("judge is not configured")
	}
	if c.JudgeRubric == "" {
		return JudgeScores{}, errors.New("judge rubric is empty")
	}
	titles := titleList(prop)
	if titles == "" {
		return JudgeScores{}, errors.New("judge cannot score an empty proposal")
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
		return JudgeScores{}, fmt.Errorf("judge call failed: %w", err)
	}
	scores, perr := parseJudge(resp.Content)
	if perr != nil {
		return JudgeScores{}, fmt.Errorf("judge output unparseable: %w (%q)", perr, truncate(resp.Content, 120))
	}
	return scores, nil
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
func parseJudge(content string) (JudgeScores, error) {
	span := extractObject(content)
	var out struct {
		Overall     *float64 `json:"overall"`
		Relevance   *float64 `json:"relevance"`
		Serendipity *float64 `json:"serendipity"`
		Reason      string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(span), &out); err != nil {
		return JudgeScores{}, err
	}
	if out.Overall == nil {
		return JudgeScores{}, errors.New("judge output is missing overall")
	}
	if out.Relevance == nil {
		return JudgeScores{}, errors.New("judge output is missing relevance")
	}
	if out.Serendipity == nil {
		return JudgeScores{}, errors.New("judge output is missing serendipity")
	}
	reason := strings.TrimSpace(out.Reason)
	if reason == "" {
		return JudgeScores{}, errors.New("judge output is missing a non-blank reason")
	}
	for _, score := range []struct {
		name  string
		value float64
	}{
		{name: "overall", value: *out.Overall},
		{name: "relevance", value: *out.Relevance},
		{name: "serendipity", value: *out.Serendipity},
	} {
		if score.value < 0 || score.value > 1 {
			return JudgeScores{}, fmt.Errorf("judge output %s score %.4g is outside 0..1", score.name, score.value)
		}
	}
	return JudgeScores{
		Overall: *out.Overall, Relevance: *out.Relevance,
		Serendipity: *out.Serendipity, Reason: reason,
	}, nil
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
