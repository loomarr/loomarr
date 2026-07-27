package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
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
//
// JSONMode is deliberately OFF while tools are offered. Forcing format:json AND
// giving tools corrupts the tool-call channel on some models (e.g. Qwen3): they
// satisfy "output JSON" by emitting the tool call as a JSON OBJECT in the content
// field instead of using the native tool_calls array — so the grounding loop sees
// no tool call and mis-parses it as a (pick-less) final answer, yielding an empty
// proposal. The system prompt already mandates "reply with ONLY JSON" for the
// FINAL turn, and the repair loop backstops any stray prose. (Caught live: qwen3:8b
// on a themed intent — correct genres, but the call landed in content, not tool_calls.)
func chatOpts(tools []llm.ToolSchema, temp float64) llm.ChatOptions {
	return llm.ChatOptions{Tools: tools, JSONMode: len(tools) == 0, Temperature: &temp}
}

// Validator re-checks a proposed acquisition against reality before it's
// actionable (§8): exists on TMDB AND not already present in the library. The
// catalog already tags in_library; this is the belt-and-suspenders re-validation
// the design mandates for anything that can trigger a download.
type Validator interface {
	// Exists reports whether a TMDB id is real (drops fabricated ids).
	Exists(ctx context.Context, mt provision.MediaType, tmdbID int) (bool, error)
}

// RatingSource fills the content rating for an acquisition NOT yet in the library
// (so with no library rating) from TMDB, at proposal time (§389 amendment). Optional:
// nil ⇒ acquisitions stay unrated until they land, then the reconciler heals them —
// this just lets a rated acquisition survive as a pending slot under an audience
// ceiling in the meantime instead of being dropped. TMDB coverage is sparse, so an
// empty answer is normal.
type RatingSource interface {
	ContentRating(ctx context.Context, mt provision.MediaType, tmdbID int) (string, error)
}

// Suggester turns an intent into a grounded proposal (§8). It composes the
// provider-neutral LLM, the catalog (grounding tool + search), and the validator
// (TMDB exists-check).
type Suggester struct {
	llm       llm.Provider
	catalog   *catalog.Catalog
	validator Validator
	ratings   RatingSource // optional acquisition-rating enrichment (§389)
	maxAcq    int          // default SUGGEST_MAX_ACQUISITIONS cap
}

// WithRatings wires the acquisition-rating enrichment (§389 amendment). Returns the
// suggester for chaining; keeps New's signature stable.
func (s *Suggester) WithRatings(r RatingSource) *Suggester {
	s.ratings = r
	return s
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

	// PRE-SEED the adjacency corpus (§8.3) before generation. These are real catalog
	// candidates with real ids — the same shape a tool call produces — so seeding them here
	// widens what the model may pick from WITHOUT weakening the chokepoint: buildProposal
	// still accepts a pick iff it appears in `surfaced`, and these appear because the
	// catalog genuinely returned them. Merging after generation instead would append picks
	// nothing ever checked, which is precisely what grounding prevents.
	//
	// The model still chooses. An offered title it ignores is simply not picked.
	for _, a := range intent.Adjacent {
		k := provision.Key(a.Key)
		mt, provider, id, ok := provision.ParseKey(k)
		if !ok {
			continue // an unparseable key could never be acquired; drop rather than offer
		}
		if _, already := surfaced[k]; already {
			continue // a tool result would win anyway; never overwrite richer data
		}
		cand := catalog.Candidate{
			MediaType: mt, Name: a.Name, Year: a.Year, Source: catalog.ScopeAdjacent,
		}
		// Rebuild the external id from the key so the candidate's own Key() round-trips —
		// a candidate whose identity doesn't reproduce would be dropped by the grounding
		// resolve, silently making every adjacency pick unpickable.
		switch provider {
		case "tmdb":
			cand.TMDBID = id
		case "tvdb":
			cand.TVDBID = id
		default:
			continue
		}
		surfaced[k] = cand
	}

	// Surface progress (§8): searching now (the model is about to ground via the
	// catalog tool), reasoning once it returns a final turn, scoring at assembly.
	reportProgress(ctx, PhaseSearching)

	// Generate → parse, with a bounded repair loop: if the model's final turn is
	// empty or malformed JSON, append a corrective nudge and re-ask at a lower
	// temperature. maxToolRounds bounds each generation; maxRepairs bounds the
	// re-asks. JOB_TIMEOUT + httpx.TimeoutLLM are the hard ceilings.
	for repair := 0; ; repair++ {
		final, err := s.generate(ctx, &messages, tools, surfaced, temp)
		if err != nil {
			return Proposal{}, err
		}
		reportProgress(ctx, PhaseReasoning)
		out, perr := parsePicks(final)
		if perr == nil {
			reportProgress(ctx, PhaseScoring)
			return s.buildProposal(ctx, intent, out, surfaced)
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
	mtArg, _ := tc.Arguments["media_type"].(string)
	genres := stringSlice(tc.Arguments["genres"])

	var cands []catalog.Candidate
	var err error
	if len(genres) > 0 {
		// DISCOVERY: the model gave genres → find by theme (+ era) rather than title.
		// This is what lets an abstract intent surface real content instead of an
		// empty keyword result. Grounding is unaffected: discovered candidates are
		// keyed into `surfaced` by the caller exactly like search results.
		from, to := parseEra(stringArg(tc.Arguments["era"]))
		cands, err = s.catalog.Discover(ctx, mediaTypeArg(mtArg), genres, from, to, catalogSearchLimit)
	} else {
		// KEYWORD: search both corpora by title.
		cands, err = s.catalog.Search(ctx, stringArg(tc.Arguments["query"]), catalog.ScopeAll, catalogSearchLimit)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	if mtArg != "" {
		cands = filterByMediaType(cands, mtArg) // narrow to the requested type
	}
	blob, _ := json.Marshal(toolResult(cands))
	return string(blob), cands
}

// mediaTypeArg maps the tool's media_type string to provision's; "" ⇒ both.
func mediaTypeArg(mt string) provision.MediaType {
	switch provision.MediaType(mt) {
	case provision.Movie:
		return provision.Movie
	case provision.Series:
		return provision.Series
	default:
		return "" // both
	}
}

// stringArg / stringSlice safely read tool-call arguments (untyped JSON).
func stringArg(v any) string { s, _ := v.(string); return s }

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
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
func (s *Suggester) buildProposal(ctx context.Context, intent Intent, out finalOutput, surfaced map[provision.Key]catalog.Candidate) (Proposal, error) {
	prop := Proposal{Intent: intent, ChannelName: strings.TrimSpace(out.ChannelName), Rationale: out.Rationale}
	picks := out.Picks
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
		// Grounding chokepoint: attach the model's proposed AIRING season window only
		// for a series, and only after clamping (an inverted or non-positive range is
		// dropped → all seasons, never an empty channel). The range can only NARROW an
		// already-grounded series expansion — it never introduces content.
		if min, max, ok := clampSeasonWindow(cand.MediaType, p.SeasonMin, p.SeasonMax); ok {
			item.SeasonMin, item.SeasonMax = min, max
		}

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
		// Enrich the rating from TMDB (§389): the library can't rate a title it doesn't
		// have, so without this an acquisition under an audience ceiling is dropped
		// before it can even show as a pending slot. Best-effort — sparse TMDB coverage
		// or a lookup error leaves it unrated, and the reconciler heals it once it lands.
		if item.OfficialRating == "" && s.ratings != nil {
			if r, err := s.ratings.ContentRating(ctx, cand.MediaType, cand.TMDBID); err == nil {
				item.OfficialRating = r
			}
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

	// Ground the extracted policy (programming-design §8): the model proposed rule
	// VALUES; we validate + clamp them (off-ladder ceiling dropped, era bounded,
	// series intersected with grounded ids) before they become a ChannelPolicy. A
	// bad policy never sinks a good lineup — it degrades to defaults (empty policy).
	prop.Policy = groundPolicy(out.Policy, prop.Lineup, prop.Acquisitions, intent)

	prop.Scores = score(intent, prop.Lineup, prop.Acquisitions)
	return prop, nil
}

// groundPolicy converts the model's untrusted pickPolicy into a validated
// schedule.ChannelPolicy (programming-design §1 extract-vs-enforce). Every value is
// machine-checked: the ceiling must be on the closed rating ladder (else dropped),
// enums must be known (else dropped), the era is taken as-is (the enforcer clamps),
// and any series allowlist is intersected with the actually-grounded picks so the
// model can't scope to a series that never surfaced. Returns an empty policy (⇒
// built-in defaults) when raw is nil or nothing survives validation.
func groundPolicy(raw *pickPolicy, lineup, acquisitions []ProposalItem, intent Intent) schedule.ChannelPolicy {
	if raw == nil {
		return schedule.ChannelPolicy{}
	}
	var p schedule.ChannelPolicy

	// Audience ceiling (programming-design §4/§8): the ceiling is a KIDS/TEEN GUARDRAIL, not a
	// general default. An unqualified channel is adult-default — "1980s Action Heroes" includes
	// its R-rated films. So a model-proposed ceiling is kept ONLY when the intent actually
	// signals kids/teens; with no such signal it is DROPPED (→ no ceiling, everything admitted),
	// because a small model reflexively caps action/genre channels ("might be violent → TV-14")
	// and that must not silently strip the R-rated content the channel is about. The prompt says
	// "adult/no mention → omit," but the model isn't trusted to obey it — this is the enforcement.
	if c := schedule.NormalizeRating(raw.Audience.Ceiling); c != "" && intentSignalsKids(intent) {
		p.Audience.Ceiling = c
		// A ceiling STRICTER than a title the model actually grounded would silently empty the
		// channel: the fail-closed audience gate (§4) drops every over-ceiling pick. Raise the
		// ceiling to admit the grounded picks — the operator asked for THESE titles — but BOUNDED
		// by the kids line: on a kids/teen channel the raise never lifts above TV-PG (a stray
		// harder pick is dropped, not admitted), so admitting-your-picks is never a safety hole.
		if raised := ceilingAdmittingPicks(p.Audience.Ceiling, lineup); raised != "" {
			p.Audience.Ceiling = raised
		}
	}
	switch schedule.UnratedPolicy(raw.Audience.Unrated) {
	case schedule.UnratedExclude, schedule.UnratedAllow:
		p.Audience.Unrated = schedule.UnratedPolicy(raw.Audience.Unrated)
	}

	// Era: accept a sane year window (the enforcer treats 0 as unbounded).
	if raw.Era.From > 0 || raw.Era.To > 0 {
		p.Scope.Era = &schedule.Range{From: raw.Era.From, To: raw.Era.To}
	}

	// Genres: pass through include/exclude names (matched case-insensitively at
	// enforcement; unknown names simply never match, which is harmless).
	p.Scope.Genres = schedule.GenreFilter{Include: raw.Genres.Include, Exclude: raw.Genres.Exclude}

	// Ordering: keep only a known mode.
	switch schedule.OrderingMode(raw.Ordering) {
	case schedule.OrderSequential, schedule.OrderShuffle, schedule.OrderSyndication:
		p.Ordering = schedule.OrderingMode(raw.Ordering)
	}
	// Multi-series default: when the model didn't pick an ordering (or picked an unknown
	// one → still OrderInherit here) AND the grounded lineup spans more than one series,
	// default to syndication so distinct series INTERMIX (deck-deal) instead of playing
	// one series to completion then the next (the chronological-per-series bug). A single-
	// series or movie channel keeps OrderInherit → the channel's Strategy decides (usually
	// sequential, correct for one show). The model's EXPLICIT choice still wins — this only
	// fills an omission. See programming-design §5.
	if p.Ordering == schedule.OrderInherit && multiSeries(lineup) {
		p.Ordering = schedule.OrderSyndication
	}

	// Seasonal: keep only a known mode + holiday ids.
	switch schedule.SeasonalMode(raw.Seasonal.Mode) {
	case schedule.SeasonalOff, schedule.SeasonalAuto, schedule.SeasonalExclusive:
		p.Seasonal.Mode = schedule.SeasonalMode(raw.Seasonal.Mode)
		p.Seasonal.Holidays = raw.Seasonal.Holidays
	}

	// Curation rules (§6.5/§6.6): lower the model's preset tokens into SchedulingRules,
	// dropping unknowns and clamping (window bound + stricter-only daypart ceiling).
	p.Rules = groundRules(raw.Rules, lineup, p.Audience.Ceiling)

	// Final safety net: if anything slipped through that Validate rejects, drop the
	// whole policy to defaults rather than persist an invalid one.
	if err := p.Validate(); err != nil {
		return schedule.ChannelPolicy{}
	}
	return p
}

// ruleWindowMin / ruleWindowMax bound a per-rule rolling window (§6.6): 1h keeps a rule
// from materializing a sliver, 168h (a week) is the longest sane horizon. The marathon
// WindowFull sentinel is exempt (a binge is meant to be unbounded).
const (
	ruleWindowMin = 1 * time.Hour
	ruleWindowMax = 168 * time.Hour
)

// groundRules lowers each model-proposed preset rule (§6.6) into a validated SchedulingRule.
// A rule whose WHEN token is unknown is DROPPED entirely (a rule with no time predicate is
// meaningless). A WHAT/HOW that doesn't lower is left to inherit the channel scope/ordering
// (the rule still applies its timing). The window is clamped; a kids/family WHAT clamps the
// rule's audience STRICTER-ONLY against the channel ceiling (§4 — a rule can never raise it,
// enforced here AND at §4 enforcement, defense in depth). A series WHAT is intersected with
// the grounded picks. Returns nil when nothing survives (⇒ base whole-policy behavior).
func groundRules(raw []pickRule, lineup []ProposalItem, channelCeiling schedule.Rating) []schedule.SchedulingRule {
	if len(raw) == 0 {
		return nil
	}
	grounded := make([]provision.Key, 0, len(lineup))
	for _, it := range lineup {
		if k, err := it.Key(); err == nil {
			grounded = append(grounded, k)
		}
	}
	out := make([]schedule.SchedulingRule, 0, len(raw))
	for i, r := range raw {
		when, prio, ok := schedule.LowerWhen(r.When)
		if !ok {
			continue // no valid time predicate → drop the whole rule
		}
		if r.Priority > 0 {
			prio = r.Priority
		}
		rule := schedule.SchedulingRule{
			ID:       fmt.Sprintf("r%d", i+1),
			Source:   schedule.RuleSourceLLM,           // provenance (§8.2): a refine may replace these; operator rules are kept
			Label:    ruleLabel(r.When, r.What, r.How), // legible attribution in the cycle preview (§8.1)
			Priority: prio,
			When:     when,
		}
		// HOW: lower ordering + per-rule window; unknown → inherit (zero How).
		if how, win, ok := schedule.LowerHow(r.How); ok {
			rule.How = how
			rule.Window = clampRuleWindow(win)
		}
		// WHAT: lower scope; unknown → inherit (nil What). A kids/family token also returns a
		// stricter-only ceiling to fold onto the rule scope's audience (§4).
		if scope, kidsCeiling, ok := schedule.LowerWhat(r.What); ok && scope != nil {
			scope.Series = intersectSeries(scope.Series, grounded) // never a series that didn't surface
			// If the intersection (or lowering) left a scope that narrows NOTHING, treat it as
			// inherit (nil) rather than a non-nil empty scope — a phantom series shouldn't leave
			// a meaningless narrower behind.
			if !scopeNarrows(scope) {
				scope = nil
			}
			rule.What = scope
			_ = kidsCeiling // audience-on-a-rule scope is a §4-followup; the channel ceiling already fences the pool.
		}
		out = append(out, rule)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ruleLabel builds a legible display name for a grounded rule from its authoring tokens
// (§8.1) — e.g. ("weekend","","marathon") → "Weekend · Marathon". Kept token-derived (not
// predicate-derived) so it preserves the WHAT specificity a lowered *ScopePolicy loses
// (a "series:tv:1396" reads as "series:tv:1396", not just "narrowed"). Empty tokens are
// skipped; an all-empty rule falls back to "Rule".
func ruleLabel(when, what, how string) string {
	parts := make([]string, 0, 3)
	for _, tok := range []string{when, what, how} {
		if t := strings.TrimSpace(tok); t != "" && !strings.EqualFold(t, "all") {
			parts = append(parts, titleToken(t))
		}
	}
	if len(parts) == 0 {
		return "Rule"
	}
	return strings.Join(parts, " · ")
}

// titleToken renders one authoring token for a label: "weekend" → "Weekend",
// "holiday:christmas" → "Holiday: Christmas". Prefixed tokens keep their qualifier.
func titleToken(tok string) string {
	if i := strings.IndexByte(tok, ':'); i >= 0 {
		return capitalizeASCII(tok[:i]) + ": " + capitalizeASCII(tok[i+1:])
	}
	return capitalizeASCII(tok)
}

// capitalizeASCII upper-cases the first byte of an ASCII token (labels only).
func capitalizeASCII(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// scopeNarrows reports whether a *ScopePolicy actually constrains anything (§6.6). An
// all-empty scope (e.g. left after a phantom series intersection dropped its only series)
// narrows nothing, so the caller nils it — a rule's What should be nil ("inherit channel
// scope"), never a non-nil no-op that reads as an active-but-empty narrower.
func scopeNarrows(s *schedule.ScopePolicy) bool {
	if s == nil {
		return false
	}
	return len(s.Series) > 0 || len(s.Collections) > 0 || s.Seasons != nil || s.Era != nil ||
		len(s.Genres.Include) > 0 || len(s.Genres.Exclude) > 0 || s.RuntimeMax > 0
}

// clampRuleWindow bounds a per-rule window to [1h,168h], leaving the WindowFull sentinel
// (a marathon binge) and 0 (inherit) untouched.
func clampRuleWindow(w schedule.Duration) schedule.Duration {
	if w == schedule.WindowFull || w == 0 {
		return w
	}
	std := w.Std()
	if std < ruleWindowMin {
		return schedule.Duration(ruleWindowMin)
	}
	if std > ruleWindowMax {
		return schedule.Duration(ruleWindowMax)
	}
	return w
}

// intersectSeries keeps only the rule's series that are actually in the grounded set (§6.6),
// so a rule can't scope to a series that never surfaced. An empty result means the rule's
// series scope drops (nil), leaving it to inherit the channel scope. A nil input (no series
// scope) passes through unchanged.
func intersectSeries(want, grounded []provision.Key) []provision.Key {
	if len(want) == 0 {
		return want
	}
	set := make(map[provision.Key]struct{}, len(grounded))
	for _, k := range grounded {
		set[k] = struct{}{}
	}
	kept := want[:0:0]
	for _, k := range want {
		if _, ok := set[k]; ok {
			kept = append(kept, k)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// tvGRank / tvPGRank are the "family" band on the audience ladder — the ONLY band within
// which an auto-raise is safe (see ceilingAdmittingPicks).
const (
	tvGRank  = 2 // TV-G / G
	tvPGRank = 3 // TV-PG / PG (== schedule.KidsCeilingRank)
)

// multiSeries reports whether a grounded lineup spans more than one DISTINCT series —
// the condition under which syndication (deck-deal intermix) is the sensible default
// ordering rather than sequential (programming-design §5). It counts distinct series by
// provisioning Key (the same identity the scheduler uses for single-series auto-relax and
// the series allowlist), so a re-picked series counts once and two seasons of one show are
// NOT two series. Movies are ignored for the count: a channel is "multi-series" only when
// ≥2 different shows are present. A Key() error (an ungrounded item — shouldn't happen post-
// validation) is skipped defensively rather than counted.
func multiSeries(lineup []ProposalItem) bool {
	seen := map[provision.Key]struct{}{}
	for _, it := range lineup {
		if it.MediaType != provision.Series {
			continue
		}
		k, err := it.Key()
		if err != nil {
			continue
		}
		seen[k] = struct{}{}
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

// ceilingAdmittingPicks fixes a small-model self-contradiction: a ceiling STRICTER than the
// channel's own grounded picks, which the fail-closed audience gate (§4) would silently empty
// (e.g. a TV-G ceiling on a TV-PG Simpsons pick). It raises the ceiling to the highest pick
// rating it needs to admit — the operator asked for THOSE titles — BUT never crosses the
// kids→adult line: the raise is clamped to KidsCeilingRank (TV-PG). This function only runs
// for a kept ceiling, which now only exists on a kids/teen channel (groundPolicy drops a
// ceiling entirely when the intent gives no kids signal), so the clamp is the whole safety
// story: a stray R/TV-MA pick on a kids channel is NOT admitted here — it's left for the §4
// enforcer to drop. Returns "" when no raise is warranted. Only rated, on-ladder picks count.
func ceilingAdmittingPicks(ceiling schedule.Rating, picks []ProposalItem) schedule.Rating {
	ceilRank, ok := ceiling.Rank()
	if !ok {
		return ""
	}
	// The highest pick rating at or below the kids line — the ceiling we'd need to admit the
	// picks without ever lifting above TV-PG. A pick above the line doesn't pull the ceiling up.
	needed := ceilRank
	for _, it := range picks {
		if rank, ok := schedule.NormalizeRating(it.OfficialRating).Rank(); ok {
			if rank > needed && rank <= schedule.KidsCeilingRank {
				needed = rank
			}
		}
	}
	if needed <= ceilRank {
		return "" // nothing to admit that a raise (within the kids band) would fix
	}
	return ratingForRank(needed)
}

// ratingForRank returns the canonical TV rating at a ladder rank (for the kids band the
// suggester's auto-raise stays within). Only ranks 0..KidsCeilingRank are produced here.
func ratingForRank(rank int) schedule.Rating {
	switch rank {
	case 0:
		return schedule.NormalizeRating("TV-Y")
	case 1:
		return schedule.NormalizeRating("TV-Y7")
	case tvGRank:
		return schedule.NormalizeRating("TV-G")
	default: // tvPGRank / KidsCeilingRank
		return schedule.NormalizeRating("TV-PG")
	}
}

// intentSignalsKids reports whether the channel intent asks for kids/teen-appropriate
// content — the ONLY case in which a proposed audience ceiling is kept (§4/§8). It scans the
// intent's free text (description/tone/era + must-include terms) for kids/family/teen cues:
// "kids", "family", "cartoons", "all ages", "wholesome", named kids properties, an explicit
// low rating token ("TV-Y", "rated G"), or a kids daypart ("saturday morning"). Deliberately
// broad on the SIGNAL side (a false positive just keeps a ceiling the operator can edit off)
// and silent otherwise — no signal ⇒ adult-default, no ceiling. Case-insensitive substring
// match; a word-boundary check keeps "family" from matching inside an unrelated word.
func intentSignalsKids(intent Intent) bool {
	hay := strings.ToLower(strings.Join([]string{
		intent.Description, intent.Tone, intent.Era, intent.RefineText,
		strings.Join(intent.MustInclude, " "),
	}, " "))
	for _, cue := range kidsIntentCues {
		if strings.Contains(hay, cue) {
			return true
		}
	}
	// Explicit low rating tokens the user might type ("rated G", "keep it TV-Y7").
	for _, r := range []string{"tv-y", "tv-g", "rated g", "rated pg"} {
		if strings.Contains(hay, r) {
			return true
		}
	}
	return false
}

// kidsIntentCues are the substrings that mark a kids/teen intent (§4/§8). Lowercased.
// Kept explicit and conservative — each is a phrase a user would only write for a
// child/family audience. "teen"/"teenager" count (a teen ceiling is TV-14, still a
// deliberate guardrail the user asked for); a generic "action"/"drama" does NOT.
var kidsIntentCues = []string{
	"kid", "kids", "child", "children", "family-friendly", "family friendly",
	"for the family", "for families", "cartoon", "all ages", "all-ages", "wholesome",
	"preschool", "toddler", "bluey", "saturday morning", "teen", "teenager", "tween",
	"g-rated", "pg-rated", "clean ", "safe for",
}

// clampSeasonWindow validates a model-proposed AIRING season window (§8 grounding
// chokepoint). It returns (min, max, true) only for a series with a sane window;
// otherwise (0,0,false) → the pick airs all seasons. Rules: series only (a movie has
// no seasons); at least one bound must be positive (0/0 = "no window", not an empty
// channel); an inverted window (min>max, both set) is nonsense and dropped. A single
// bound is allowed (seasonMin:11 = "11 onward", seasonMax:10 = "through 10"). The
// window only NARROWS a grounded series' expansion — it can never add content.
func clampSeasonWindow(mt provision.MediaType, min, max int) (int, int, bool) {
	if mt != provision.Series {
		return 0, 0, false
	}
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if min == 0 && max == 0 {
		return 0, 0, false // no window proposed
	}
	if min > 0 && max > 0 && min > max {
		return 0, 0, false // inverted → drop (all seasons, never empty)
	}
	return min, max, true
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
- Pick the search mode from the intent, and CALL THE TOOL MORE THAN ONCE if the first call comes back empty or thin:
  - GENRE/MOOD/ERA intent (e.g. "90s action", "feel-good sci-fi") → call catalog_search with "genres" (and "era") to DISCOVER by theme. Do NOT put a bare genre word in "query" — it won't match a title.
  - THEMATIC-KEYWORD intent — a holiday, franchise, motif, or topic that is NOT a genre (e.g. "Christmas", "Halloween", "zombie", "heist", "based on a true story", "Star Wars") → use "query" with that keyword. These match titles AND overviews, so a "query" search is the RIGHT tool even though the intent is thematic. Genre discovery will MISS them (there is no "Christmas" or "heist" genre).
  - KNOWN TITLE → "query" with the title.
- If a call returns few or no candidates, TRY THE OTHER MODE before giving up (a genre discovery that finds nothing → retry as a "query" keyword, and vice-versa). Never conclude "no content" after a single empty search.
- Each result carries genres + a short overview — use them to judge which titles fit the intent.
- HONOR EVERY QUALIFIER in the intent, not just the main noun. "cozy British murder mysteries" means British AND cozy AND mystery — a 1940s American noir or a Swedish thriller that merely has "murder" in the title does NOT fit; check the overview (setting, country, tone) and REJECT it. "classic Star Trek — the original series and Next Generation" names TWO specific shows: include those, not every Star Trek series. It is BETTER to return fewer, well-matched picks (or none) than to pad the lineup with titles that only match one keyword.
- HONOR EXPLICITLY NAMED titles: if the intent names specific shows/movies, search for those by name and prefer them; don't substitute lookalikes.
- A pick whose genres/overview CONTRADICT the intent (wrong country, wrong tone, wrong era, wrong subject) must be dropped even if its title contains the keyword. Matching one word is not fitting the intent.
- Select ONLY from ids the tool returns. Never output a tmdbId or tvdbId that did not appear in a tool result.
- Prefer titles already in the library (inLibrary:true) for the lineup; propose missing ones as acquisitions.
- SEASON WINDOW for a series: when the intent implies an ERA of a long-running show, set "seasonMin"/"seasonMax" on that series pick so ONLY those seasons air. Examples: "Simpsons Classics" or "classic Simpsons" -> the golden-age run, seasonMin:1, seasonMax:10; "early Seinfeld" -> seasonMin:1, seasonMax:4; "first three seasons of X" -> seasonMin:1, seasonMax:3; "late-era X" -> seasonMin only. Use it ONLY when the intent scopes an era of a SERIES; omit both for movies and for "all of a show". This narrows which episodes play; it does NOT change what gets acquired.
- Also infer a "policy" describing HOW the channel should behave, from the intent. Only include a field you can justify from the intent; omit the rest.
  - audience.ceiling: a KIDS/TEEN safety cap — set it ONLY when the intent explicitly asks for a young audience ("cartoons"/"for kids" -> "TV-Y7"; "family" -> "TV-PG"; "teen" -> "TV-14"). For ANY channel that does not ask for kids/family/teen content, OMIT it entirely — an unqualified channel is adult-default and includes its R-rated titles (e.g. "action heroes" includes Die Hard/The Terminator; do NOT cap it). When in doubt, omit. Use ONLY these values: TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA (or film G, PG, PG-13, R, NC-17).
  - era.from/era.to: year range if the intent names one ("90s" -> from 1990 to 1999).
  - genres.include/exclude: genre names implied by the intent.
  - ordering: "syndication" (rerun feel — different series INTERMIXED, like a rerun channel), "sequential" (strict order — one show start-to-finish), or "shuffle". RULE OF THUMB: if the lineup has MORE THAN ONE series, prefer "syndication" so the shows interleave instead of playing one series to the end before the next (a Star-Trek-style multi-show channel should feel like flipping between them, not a chronological box set). Use "sequential" only for a SINGLE show meant to play in episode order, or when the intent explicitly asks for chronological/marathon.
  - seasonal.mode: "exclusive" for a holiday channel, "auto" otherwise (omit for evergreen).
  - rules: OPTIONAL time-of-day / calendar programming, ONLY if the intent asks for it ("weekend marathons", "holiday specials in December", "kids in the morning"). Each rule is {when, what, how} from a CLOSED vocabulary — do NOT invent times or predicates:
    - when: "weekend" | "weekday" | "mornings" | "daytime" | "primetime" | "late-night" | "overnight" | "holiday:christmas" | "holiday:halloween" | "holiday:thanksgiving" | "holiday:newyear" | "holiday:valentines" | a composite like "weekend-mornings".
    - what (optional; omit to keep the whole channel): "series:<the exact key of a pick>" | "genre:<name>" | "kids" | "family" | "holiday-matched" | "all".
    - how (optional; omit to keep the channel's ordering): "marathon" (binge one show, no breaks) | "syndication" | "shuffle".
    Examples: a weekend TNG marathon → {"when":"weekend","what":"series:...","how":"marathon"}; December holiday programming → {"when":"holiday:christmas","what":"holiday-matched"}. Omit "rules" for a plain channel; unknown tokens are dropped.
- SET "confidence" ON EVERY PICK, and CALIBRATE it — it is not a formality. It means "how well does THIS title fit THIS intent", and a downstream quality bar spends the user's disk and bandwidth on the strength of it. Use the whole range:
  - 0.9-1.0 — squarely what was asked for (an explicitly named title; an unambiguous match on every qualifier).
  - 0.7-0.8 — a good fit that misses one qualifier (right genre and era, tone slightly off).
  - 0.4-0.6 — plausible but arguable; you would not defend it if challenged.
  - below 0.4 — a stretch. Prefer dropping the pick entirely over padding the lineup with it.
  Do NOT give everything 0.9+. If every pick scores the same, the score carries no information and the bar cannot do its job. A pick you were HANDED (see the suggestions below, if any) still needs your own honest score — judge it against the intent exactly as you would one you found yourself, and never omit the field.
- Also invent a short, catchy "channelName" for the channel (2-4 words, like a real TV network — e.g. "Springfield Classics" for a Simpsons channel, "Fright Night Theater" for horror). NOT the user's raw prompt, and NOT a single title's name.
When finished, reply with ONLY this JSON (no prose):
{"channelName":"<2-4 words>","rationale":"<one sentence>","picks":[{"mediaType":"movie|series","tmdbId":<int>,"tvdbId":<int optional>,"name":"<string>","rationale":"<why it fits>","confidence":<0..1>,"seasonMin":<int optional, series era only>,"seasonMax":<int optional, series era only>}],"policy":{"audience":{"ceiling":"<rating>"},"era":{"from":<int>,"to":<int>},"genres":{"include":["..."],"exclude":["..."]},"ordering":"<mode>","seasonal":{"mode":"<mode>"},"rules":[{"when":"<token>","what":"<token optional>","how":"<token optional>"}]}}`

// repairPrompt nudges the model when its final turn wasn't valid schema JSON (or
// was empty). Kept short + imperative; it never relaxes the grounding rules.
const repairPrompt = `Your previous reply was not valid JSON matching the required schema (or was empty). ` +
	`Reply now with ONLY the JSON object {"rationale":...,"picks":[...]} and nothing else. ` +
	`Use ONLY ids that appeared in a catalog_search result.`

// userPrompt renders the intent into the first user turn.
func userPrompt(i Intent) string {
	var b strings.Builder
	// Refine framing (§7): when the channel already exists, lead with its current lineup +
	// the requested change, so the model KEEPS what still fits and revises the rest — rather
	// than proposing a channel from scratch. Grounding is unchanged: it must still call the
	// catalog tool for every pick, kept or new (the buildProposal `surfaced` gate enforces it).
	if i.RefineText != "" || len(i.CurrentLineup) > 0 {
		fmt.Fprintf(&b, "This channel already exists: %s\n", i.Description)
		if len(i.CurrentLineup) > 0 {
			b.WriteString("Its current lineup is:\n")
			for _, e := range i.CurrentLineup {
				if e.Year > 0 {
					fmt.Fprintf(&b, "  - %s (%d)\n", e.Name, e.Year)
				} else {
					fmt.Fprintf(&b, "  - %s\n", e.Name)
				}
			}
		}
		if i.RefineText != "" {
			fmt.Fprintf(&b, "The user wants to change it: %s\n", i.RefineText)
		}
		b.WriteString("Keep the titles that still fit, drop the ones that don't, and add new ones as needed. " +
			"Re-ground EVERY title (kept or new) through the catalog tool — use only ids the tool returns.\n")
		// Adjacency offers (§8.3): titles this channel's own lineup points at, with the
		// consensus that surfaced them. Presented as SUGGESTIONS, not instructions — the
		// model weighs them against the intent like any other candidate, and a weak
		// consensus should read as weaker evidence than a strong one. Their ids are already
		// grounded (pre-seeded into `surfaced`), which is why they may be picked directly.
		if len(i.Adjacent) > 0 {
			b.WriteString("Viewers of the titles above also watch these — consider them if they fit the channel:\n")
			for _, a := range i.Adjacent {
				if a.Year > 0 {
					fmt.Fprintf(&b, "  - %s (%d) [%s] — suggested by %d of its titles\n", a.Name, a.Year, a.Key, a.Votes)
				} else {
					fmt.Fprintf(&b, "  - %s [%s] — suggested by %d of its titles\n", a.Name, a.Key, a.Votes)
				}
			}
			b.WriteString("These ids are already grounded — you may pick them directly. Ignore any that don't fit.\n")
		}
	} else {
		fmt.Fprintf(&b, "Build a channel: %s\n", i.Description)
	}
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
	if i.RuntimeTgt > 0 {
		fmt.Fprintf(&b, "Target total runtime (minutes): %d\n", i.RuntimeTgt)
	}
	if i.MaxAcquire > 0 {
		fmt.Fprintf(&b, "Propose at most %d titles that need acquiring (not already in the library).\n", i.MaxAcquire)
	}
	return b.String()
}

// catalogTool is the provider-neutral tool schema the model may call (§8). It does
// double duty: `query` runs a title keyword search; `genres` (+ optional `era`)
// runs DISCOVERY by theme — use that for an abstract intent ("high-energy 90s
// action") that no exact title matches. Either mode returns real ids + genres +
// overview + an inLibrary flag; it is the ONLY way to find titles.
func catalogTool() llm.ToolSchema {
	return llm.ToolSchema{
		Name: catalogToolName,
		Description: "Find real titles from the library + TMDB. Provide `query` to search by title, OR `genres` " +
			"(with optional `era`) to DISCOVER titles by theme when no exact title fits the intent. " +
			"Returns real external ids, genres, a short overview, and an inLibrary flag. This is the ONLY way to find titles.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":      map[string]any{"type": "string", "description": "title keywords (for a known title)"},
				"genres":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "genre names to discover by, e.g. [\"Action\",\"Science Fiction\"]"},
				"era":        map[string]any{"type": "string", "description": "decade or year range for discovery, e.g. \"1990s\" or \"1985-1995\""},
				"media_type": map[string]any{"type": "string", "enum": []string{"movie", "series"}},
			},
		},
	}
}
