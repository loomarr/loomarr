package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/provision"
)

// catalogToolName is the single tool the model may call. The grounding guarantee
// (§8) is enforced structurally: this is the ONLY way the model can name a title,
// and it returns real ids from the real library + TMDB.
const catalogToolName = "catalog_search"

// maxToolRounds bounds the tool-call loop so a misbehaving model can't spin
// forever (defense-in-depth; JOB_TIMEOUT is the outer bound).
const maxToolRounds = 6

// catalogSearchLimit is how many candidates the catalog tool returns per call.
// Higher than the old hardcoded 12 so an abstract/themed intent surfaces enough
// rows for the model to ground a real lineup before truncation, without flooding
// a small local model's context.
const catalogSearchLimit = 24

// groundedTemp is the sampling temperature for the grounded/JSON turns. Low so the
// model adheres to the tool-call + JSON-schema contract rather than getting
// creative — tool-calling and structured output want determinism, not variety.
var groundedTemp = 0.2

// chatOpts builds the per-turn ChatOptions with the tools + grounded sampling.
// temp lets the repair loop lower it further on a retry.
func chatOpts(tools []llm.ToolSchema, temp float64) llm.ChatOptions {
	return llm.ChatOptions{Tools: tools, JSONMode: true, Temperature: &temp}
}

// Validator re-checks a proposed acquisition against reality before it's
// actionable (§8): exists on TMDB AND not already present in the library. The
// catalog already tags in_library; this is the belt-and-suspenders re-validation
// the design mandates for anything that can trigger a download.
type Validator interface {
	// Exists reports whether a TMDB id is real (drops fabricated ids).
	Exists(ctx context.Context, mt provision.MediaType, tmdbID int) (bool, error)
}

// Suggester turns an intent into a grounded proposal (§8). It composes the
// provider-neutral LLM, the catalog (grounding tool + search), and the validator
// (TMDB exists-check).
type Suggester struct {
	llm       llm.Provider
	catalog   *catalog.Catalog
	validator Validator
	maxAcq    int // default SUGGEST_MAX_ACQUISITIONS cap
}

// New builds a Suggester. maxAcq is the default acquisition cap (§8 quota).
func New(provider llm.Provider, cat *catalog.Catalog, v Validator, maxAcq int) *Suggester {
	if maxAcq <= 0 {
		maxAcq = 10
	}
	return &Suggester{llm: provider, catalog: cat, validator: v, maxAcq: maxAcq}
}

// Suggest runs the grounded generation for an intent (§8). It drives the tool-use
// loop (model → catalog_search → results → model → final JSON), parses the
// model's picks, resolves each against the catalog (grounding: drop anything not
// resolvable to a real id), re-validates acquisitions (exists on TMDB), and
// scores the result deterministically. Returns a Proposal ready for the approval
// queue — NEVER auto-executed (§8 human-in-the-loop).
// maxRepairs bounds the JSON-repair re-asks after the tool loop produces a final
// turn that's empty or not valid schema JSON. Small — a model that can't produce
// the schema in a couple of nudges won't in ten. Separate from maxToolRounds.
const maxRepairs = 2

func (s *Suggester) Suggest(ctx context.Context, intent Intent) (Proposal, error) {
	messages := []llm.Message{
		{Role: llm.System, Content: systemPrompt},
		{Role: llm.User, Content: userPrompt(intent)},
	}
	tools := []llm.ToolSchema{catalogTool()}

	// Track every candidate the tool surfaced this run, keyed by provisioning key.
	// A pick is grounded IFF it matches one of these — the model cannot smuggle in
	// an id the tool never returned. Threaded across the tool loop AND repair
	// re-asks, so grounding holds even when the final JSON is retried.
	surfaced := map[provision.Key]catalog.Candidate{}
	temp := groundedTemp

	// Generate → parse, with a bounded repair loop: if the model's final turn is
	// empty or malformed JSON, append a corrective nudge and re-ask at a lower
	// temperature. maxToolRounds bounds each generation; maxRepairs bounds the
	// re-asks. JOB_TIMEOUT + httpx.TimeoutLLM are the hard ceilings.
	for repair := 0; ; repair++ {
		final, err := s.generate(ctx, &messages, tools, surfaced, temp)
		if err != nil {
			return Proposal{}, err
		}
		picks, rationale, perr := parsePicks(final)
		if perr == nil {
			return s.buildProposal(ctx, intent, picks, rationale, surfaced)
		}
		if repair >= maxRepairs {
			return Proposal{}, fmt.Errorf("suggester: model output not valid after %d repairs: %w", maxRepairs, perr)
		}
		// Nudge the model to fix its output, and turn the temperature down further
		// so it adheres to the schema rather than getting creative.
		messages = append(messages, llm.Message{Role: llm.User, Content: repairPrompt})
		temp = temp / 2
	}
}

// generate runs the tool-call loop until the model returns a final (non-tool)
// turn, appending assistant/tool messages to *messages and recording surfaced
// candidates for grounding. Returns the final content (possibly empty — the
// caller's repair loop handles that).
func (s *Suggester) generate(ctx context.Context, messages *[]llm.Message, tools []llm.ToolSchema, surfaced map[provision.Key]catalog.Candidate, temp float64) (string, error) {
	for round := 0; round < maxToolRounds; round++ {
		resp, err := s.llm.Chat(ctx, *messages, chatOpts(tools, temp))
		if err != nil {
			return "", fmt.Errorf("llm chat: %w", err)
		}
		if resp.WantsTools() {
			*messages = append(*messages, assistantToolCallMsg(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				result, cands := s.runTool(ctx, tc)
				for _, c := range cands {
					if k, err := c.Key(); err == nil {
						surfaced[k] = c
					}
				}
				*messages = append(*messages, llm.Message{
					Role: llm.Tool, Content: result, ToolCallID: tc.ID,
				})
			}
			continue
		}
		return resp.Content, nil
	}
	// Ran out of tool rounds without a final turn — treat as empty (repairable).
	return "", nil
}

// runTool executes a model tool call. Only catalog_search is honored; anything
// else returns an error result the model can react to (defense against a model
// inventing a tool). Returns the JSON result string AND the candidates (so the
// suggester can track what was surfaced for grounding).
func (s *Suggester) runTool(ctx context.Context, tc llm.ToolCall) (string, []catalog.Candidate) {
	if tc.Name != catalogToolName {
		return fmt.Sprintf(`{"error":"unknown tool %q; only %s is available"}`, tc.Name, catalogToolName), nil
	}
	query, _ := tc.Arguments["query"].(string)
	// Search both corpora (library + TMDB), then honor media_type as a post-filter
	// when the model asked for a specific type — so "find me series" doesn't return
	// movies. Grounding is unaffected: whatever survives is still keyed into
	// `surfaced` by the caller exactly as before.
	cands, err := s.catalog.Search(ctx, query, catalog.ScopeAll, catalogSearchLimit)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	if mt, ok := tc.Arguments["media_type"].(string); ok && mt != "" {
		cands = filterByMediaType(cands, mt)
	}
	blob, _ := json.Marshal(toolResult(cands))
	return string(blob), cands
}

// filterByMediaType keeps only candidates matching the requested type ("movie" or
// "series"); an unrecognized value is ignored (returns all — never hides content
// from the model on a bad hint).
func filterByMediaType(cands []catalog.Candidate, mt string) []catalog.Candidate {
	want := provision.MediaType(mt)
	if want != provision.Movie && want != provision.Series {
		return cands
	}
	out := cands[:0:0]
	for _, c := range cands {
		if c.MediaType == want {
			out = append(out, c)
		}
	}
	return out
}

// buildProposal turns the model's picks into a validated, grounded, scored
// Proposal. This is the grounding chokepoint: a pick survives ONLY if it matches
// a candidate the tool actually surfaced (real id), and acquisitions must also
// pass the exists re-validation. Unresolvable picks are dropped, never actioned.
func (s *Suggester) buildProposal(ctx context.Context, intent Intent, picks []pick, rationale string, surfaced map[provision.Key]catalog.Candidate) (Proposal, error) {
	prop := Proposal{Intent: intent, Rationale: rationale}
	acqCount := 0
	maxAcq := s.maxAcq
	if intent.MaxAcquire > 0 && intent.MaxAcquire < maxAcq {
		maxAcq = intent.MaxAcquire
	}

	for _, p := range picks {
		key := p.key()
		if key == "" {
			continue // no usable id → not grounded, drop
		}
		cand, ok := surfaced[provision.Key(key)]
		if !ok {
			continue // GROUNDING: the model named an id the tool never returned — drop it
		}
		item := fromCandidate(cand, p.Rationale, p.Confidence)

		if cand.InLibrary {
			prop.Lineup = append(prop.Lineup, item)
			continue
		}
		// Acquisition: re-validate it exists on TMDB (§8) and respect the cap.
		if acqCount >= maxAcq {
			prop.Alternates = append(prop.Alternates, item) // over-cap picks become alternates
			continue
		}
		exists, err := s.validator.Exists(ctx, cand.MediaType, cand.TMDBID)
		if err != nil {
			return Proposal{}, fmt.Errorf("validate acquisition %s: %w", cand.Name, err)
		}
		if !exists {
			continue // fabricated/withdrawn id → drop
		}
		prop.Acquisitions = append(prop.Acquisitions, item)
		acqCount++
	}

	// A proposal with nothing grounded (every pick was fabricated/withdrawn, or the
	// search surfaced no themed content) is a real failure, not a success. Return a
	// sentinel so the worker fails the job with a clear reason — instead of
	// persisting an empty "submitted" proposal and caching it for 24h (which made the
	// failure both silent and sticky). Grounding is unaffected: this only decides
	// what a legitimately-empty result does.
	if len(prop.Lineup)+len(prop.Acquisitions)+len(prop.Alternates) == 0 {
		return Proposal{}, ErrNoGroundedTitles
	}

	prop.Scores = score(intent, prop.Lineup, prop.Acquisitions)
	return prop, nil
}

// ErrNoGroundedTitles is returned when a run produced no grounded picks at all —
// the intent surfaced no themed, real content. The worker fails the job with this
// (a clear operator-facing reason), and it is NOT cached, so a re-submit re-runs.
var ErrNoGroundedTitles = errors.New("suggester: no grounded titles found for this intent")

// assistantToolCallMsg builds the assistant message that carries the model's
// tool-call requests back into the conversation.
func assistantToolCallMsg(calls []llm.ToolCall) llm.Message {
	return llm.Message{Role: llm.Assistant, ToolCalls: calls}
}

// systemPrompt forbids invention and constrains the model to tool results (§8).
const systemPrompt = `You are Loomarr's channel planner. You build TV channels from real content only.
RULES:
- You MUST NOT invent titles. To find any title, call the catalog_search tool.
- Select ONLY from ids the tool returns. Never output a tmdbId or tvdbId that did not appear in a tool result.
- Prefer titles already in the library (inLibrary:true) for the lineup; propose missing ones as acquisitions.
When finished, reply with ONLY this JSON (no prose):
{"rationale":"<one sentence>","picks":[{"mediaType":"movie|series","tmdbId":<int>,"tvdbId":<int optional>,"name":"<string>","rationale":"<why it fits>","confidence":<0..1>}]}`

// repairPrompt nudges the model when its final turn wasn't valid schema JSON (or
// was empty). Kept short + imperative; it never relaxes the grounding rules.
const repairPrompt = `Your previous reply was not valid JSON matching the required schema (or was empty). ` +
	`Reply now with ONLY the JSON object {"rationale":...,"picks":[...]} and nothing else. ` +
	`Use ONLY ids that appeared in a catalog_search result.`

// userPrompt renders the intent into the first user turn.
func userPrompt(i Intent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Build a channel: %s\n", i.Description)
	if i.Era != "" {
		fmt.Fprintf(&b, "Era: %s\n", i.Era)
	}
	if i.Tone != "" {
		fmt.Fprintf(&b, "Tone: %s\n", i.Tone)
	}
	if len(i.MustInclude) > 0 {
		fmt.Fprintf(&b, "Must include: %s\n", strings.Join(i.MustInclude, ", "))
	}
	if len(i.MustExclude) > 0 {
		fmt.Fprintf(&b, "Must exclude: %s\n", strings.Join(i.MustExclude, ", "))
	}
	return b.String()
}

// catalogTool is the provider-neutral tool schema the model may call (§8).
func catalogTool() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        catalogToolName,
		Description: "Search the real library + TMDB for titles. Returns real external ids and an inLibrary flag. This is the ONLY way to find titles.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":      map[string]any{"type": "string", "description": "search terms"},
				"media_type": map[string]any{"type": "string", "enum": []string{"movie", "series"}},
			},
			"required": []string{"query"},
		},
	}
}
