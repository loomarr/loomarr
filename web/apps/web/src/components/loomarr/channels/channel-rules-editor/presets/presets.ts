import type { HowVocab, RuleOrdering, ScopePolicy, WhatVocab, WhenVocab } from "@loomarr/api";

// The rule authoring vocabulary is now SERVED by the backend (GET /v1/programming/vocabulary,
// §6.6/§8.1) and consumed via a prop — this file no longer hand-mirrors the enumerable
// WHEN/WHAT/HOW tables from internal/schedule/presets.go (the old byte-for-byte mirror + its
// drift hazard are gone). What stays here is only the PARAMETRIC lowering the endpoint can't
// enumerate (series:<key>, genre:<name>, era:<range> are composed from the channel's own
// lineup) plus the reverse lookups, all parameterized BY the served vocabulary so there is a
// single source of truth.
//
// ⚠ The served arrays are NO LONGER nullable on the wire (V53b — `huma.DefaultArrayNullable` is
// false and a guard proves no handler returns nil). This comment used to say they were, and the
// coalescing below existed for that. The coalescing STAYS, for a different and still-real reason:
// the vocabulary arrives from a query, so it is `undefined` until the fetch resolves, and a
// not-yet-loaded vocabulary should read as "no options" rather than crash. Null is gone; unloaded
// is not.
//
// ⚠ The generated `VocabularyWhen`/`VocabularyWhat`/`VocabularyHow` aliases these signatures used
// are gone too, and that is a side effect worth knowing: orval only emitted them because
// `X[] | null` needed a name. Non-nullable arrays inline to `WhenVocab[]`, so the aliases stopped
// being generated — nothing was renamed, they simply had no reason to exist.

const arr = <T>(x: readonly T[] | null | undefined): readonly T[] => x ?? [];

// ---- WHEN --------------------------------------------------------------------------------

const whenOptions = (when: WhenVocab[]): { value: string; label: string }[] =>
  arr<WhenVocab>(when).map((p) => ({ value: p.token, label: p.label }));
const whenShortLabels = (when: WhenVocab[]): { value: string; label: string }[] =>
  arr<WhenVocab>(when).map((p) => ({ value: p.token, label: p.shortLabel }));

// lowerWhen — token → the predicate + priority the BE lowers it to (from the served
// vocabulary, so the editor's preview matches the server exactly).
const lowerWhen = (
  when: WhenVocab[],
  token: string,
): { predicate: WhenVocab["predicate"]; priority: number } | undefined => {
  const p = arr<WhenVocab>(when).find((w) => w.token === token);
  return p ? { predicate: p.predicate, priority: p.priority } : undefined;
};

// tokenForWhen — reverse lookup: which served token produced this rule's predicate, so the
// editor preselects the right option. No match (hand-authored/composite) → "" ("Custom").
const tokenForWhen = (when: WhenVocab[], w: WhenVocab["predicate"] | undefined): string => {
  if (!w) return "";
  const match = arr<WhenVocab>(when).find(
    (p) =>
      Boolean(p.predicate.weekend) === Boolean(w.weekend) &&
      Boolean(p.predicate.weekday) === Boolean(w.weekday) &&
      (p.predicate.hourFrom ?? 0) === (w.hourFrom ?? 0) &&
      (p.predicate.hourTo ?? 0) === (w.hourTo ?? 0) &&
      (p.predicate.holiday ?? "") === (w.holiday ?? ""),
  );
  return match?.token ?? "";
};

// ---- WHAT (served static + parametric prefixes here) ------------------------------------

const whatStaticOptions = (what: WhatVocab[]): { value: string; label: string }[] =>
  arr<WhatVocab>(what).map((w) => ({ value: w.token, label: w.label }));

// lowerWhat — token → scope. Parametric prefixes (series:/genre:/era:) are parsed here (the
// endpoint can't enumerate them); the static tokens' scopes come from the served vocabulary.
const lowerWhat = (what: WhatVocab[], token: string): { scope: ScopePolicy | undefined } | undefined => {
  const t = token.trim();
  if (t.startsWith("series:")) {
    const key = t.slice("series:".length).trim();
    return key ? { scope: { series: [key] } } : undefined;
  }
  if (t.startsWith("genre:")) {
    const g = t.slice("genre:".length).trim();
    return g ? { scope: { genres: { include: [g] } } } : undefined;
  }
  if (t.startsWith("era:")) {
    const parsed = parseEraToken(t.slice("era:".length));
    return parsed ? { scope: { era: parsed } } : undefined;
  }
  const entry = arr<WhatVocab>(what).find((w) => w.token === (t === "" ? "all" : t));
  if (!entry) return undefined;
  return { scope: entry.scope ?? undefined };
};

// tokenForWhat — reverse lookup for a rule's scope. Parametric shapes recognized directly;
// otherwise it matches a served static preset's scope.
const tokenForWhat = (what: WhatVocab[], scope: ScopePolicy | null | undefined): string => {
  if (!scope) return "all";
  const series = scope.series;
  if (series && series.length > 0) return `series:${series[0]}`;
  const include = scope.genres?.include ?? [];
  const match = arr<WhatVocab>(what).find(
    (w) => include.length > 0 && arraysEqual(w.scope?.genres?.include ?? [], include),
  );
  if (match) return match.token;
  if (include.length === 1 && !scope.genres?.exclude?.length && !scope.era && !scope.seasons) {
    return `genre:${include[0]}`;
  }
  if (scope.era && (scope.era.from || scope.era.to)) {
    return `era:${scope.era.from ?? ""}-${scope.era.to ?? ""}`;
  }
  return "";
};

const arraysEqual = (a: string[], b: string[]): boolean =>
  a.length === b.length && a.every((v, i) => v === b[i]);

// parseEraToken — "1990-1999" / "1990-" / "-1999" → a Range (parametric; stays FE-side).
const parseEraToken = (s: string): { from?: number; to?: number } | undefined => {
  const trimmed = s.trim();
  const idx = trimmed.indexOf("-");
  if (idx === -1) return undefined;
  const fromStr = trimmed.slice(0, idx).trim();
  const toStr = trimmed.slice(idx + 1).trim();
  const range: { from?: number; to?: number } = {};
  if (fromStr !== "") {
    const n = Number(fromStr);
    if (!Number.isInteger(n) || n <= 0) return undefined;
    range.from = n;
  }
  if (toStr !== "") {
    const n = Number(toStr);
    if (!Number.isInteger(n) || n <= 0) return undefined;
    range.to = n;
  }
  if (range.from === undefined && range.to === undefined) return undefined;
  return range;
};

// ---- HOW ---------------------------------------------------------------------------------

const howOptions = (how: HowVocab[]): { value: string; label: string }[] =>
  arr<HowVocab>(how).map((p) => ({ value: p.token, label: p.label }));
const howShortLabels = (how: HowVocab[]): { value: string; label: string }[] =>
  arr<HowVocab>(how).map((p) => ({ value: p.token, label: p.shortLabel }));

// lowerHow — token → the RuleOrdering + whether it pins the full run (marathon), from the
// served vocabulary. `window: "full"` mirrors schedule.WindowFull.
const lowerHow = (
  how: HowVocab[],
  token: string,
): { ordering: RuleOrdering; window?: "full" } | undefined => {
  const p = arr<HowVocab>(how).find((h) => h.token === token);
  if (!p) return undefined;
  return { ordering: p.ordering, window: p.windowFull ? "full" : undefined };
};

// tokenForHow — reverse lookup for a rule's RuleOrdering. "" ("Custom") when nothing matches.
const tokenForHow = (how: HowVocab[], ordering: RuleOrdering | undefined): string => {
  if (!ordering?.ordering) return "";
  const match = arr<HowVocab>(how).find(
    (p) =>
      p.ordering.ordering === ordering.ordering &&
      Boolean(p.ordering.noBreaks) === Boolean(ordering.noBreaks) &&
      (p.ordering.separation?.blockMax ?? 0) === (ordering.separation?.blockMax ?? 0),
  );
  return match?.token ?? "";
};

export {
  howOptions,
  howShortLabels,
  lowerHow,
  lowerWhat,
  lowerWhen,
  tokenForHow,
  tokenForWhat,
  tokenForWhen,
  whatStaticOptions,
  whenOptions,
  whenShortLabels,
};
