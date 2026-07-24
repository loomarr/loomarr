import type { RuleOrdering, ScopePolicy, WhenPredicate } from "@loomarr/api";

// The FE mirror of internal/schedule/presets.go (programming-design §6.6 — the closed
// authoring vocabulary). Keep this table BYTE-FOR-BYTE in sync with presets.go: same
// tokens, same predicates/scopes/orderings, same default priorities. It exists so a
// hand-authored rule in this editor lowers to the identical SchedulingRule shape an
// LLM-authored one would (§8.1) — a UX convenience for building the picker + a live
// preview label, NOT the source of truth. The backend re-validates on every write
// (unknown tokens dropped, window clamped, audience stricter-only, §6.6 grounding) —
// so a bug here degrades to "the picker offered something the server then dropped",
// never a safety hole. If presets.go changes, update this file in the same PR.

// ---- WHEN tokens (WhenPredicate) ----------------------------------------------------

interface WhenPreset {
  token: string;
  label: string;
  // A terser form of `label` for the computed row caption (label.ts) — the Select's own
  // item text carries the hour-range explanation, so the caption doesn't repeat it.
  shortLabel: string;
  predicate: WhenPredicate;
  priority: number;
}

// Holiday ids match the §6 builtinCalendar (knownHoliday in presets.go). Kept as a
// small closed list here too — the BE is the real gate, this is just what the picker
// offers.
const HOLIDAY_IDS = ["christmas", "halloween", "thanksgiving", "newyear", "valentines"] as const;

const HOLIDAY_LABELS: Record<(typeof HOLIDAY_IDS)[number], string> = {
  christmas: "Christmas",
  halloween: "Halloween",
  thanksgiving: "Thanksgiving",
  newyear: "New Year",
  valentines: "Valentine's Day",
};

// The base (non-holiday, non-composite) WHEN tokens — mirrors lowerBaseWhen exactly,
// including its default priorities (weekend/weekday=20, dayparts=30/40).
const WHEN_PRESETS: WhenPreset[] = [
  { token: "weekend", label: "Weekend", shortLabel: "Weekend", predicate: { weekend: true }, priority: 20 },
  { token: "weekday", label: "Weekday", shortLabel: "Weekday", predicate: { weekday: true }, priority: 20 },
  {
    token: "mornings",
    label: "Mornings (6–10)",
    shortLabel: "Mornings",
    predicate: { hourFrom: 6, hourTo: 10 },
    priority: 30,
  },
  {
    token: "daytime",
    label: "Daytime (10–17)",
    shortLabel: "Daytime",
    predicate: { hourFrom: 10, hourTo: 17 },
    priority: 30,
  },
  {
    token: "primetime",
    label: "Primetime (20–23)",
    shortLabel: "Primetime",
    predicate: { hourFrom: 20, hourTo: 23 },
    priority: 40,
  },
  {
    token: "late-night",
    label: "Late night (23–2)",
    shortLabel: "Late night",
    predicate: { hourFrom: 23, hourTo: 2 },
    priority: 40,
  },
  {
    token: "overnight",
    label: "Overnight (2–6)",
    shortLabel: "Overnight",
    predicate: { hourFrom: 2, hourTo: 6 },
    priority: 40,
  },
  ...HOLIDAY_IDS.map((id) => ({
    token: `holiday:${id}`,
    label: HOLIDAY_LABELS[id],
    shortLabel: HOLIDAY_LABELS[id],
    predicate: { holiday: id },
    priority: 60,
  })),
];

// The options the WHEN select offers, grouped only visually — the underlying value is
// always the flat token string.
const WHEN_OPTIONS: { value: string; label: string }[] = WHEN_PRESETS.map((p) => ({
  value: p.token,
  label: p.label,
}));

// WHEN_SHORT_LABELS — token → terse label, for the computed row caption (label.ts).
const WHEN_SHORT_LABELS: { value: string; label: string }[] = WHEN_PRESETS.map((p) => ({
  value: p.token,
  label: p.shortLabel,
}));

// lowerWhen — the FE mirror of LowerWhen (presets.go). No composite ("weekend-mornings")
// support here: the editor offers only the atomic tokens (composites are an LLM/authoring
// convenience the BE still accepts, but the picker doesn't need to compose them itself).
const lowerWhen = (token: string): { predicate: WhenPredicate; priority: number } | undefined => {
  const preset = WHEN_PRESETS.find((p) => p.token === token);
  if (!preset) return undefined;
  return { predicate: preset.predicate, priority: preset.priority };
};

// tokenForWhen — the reverse lookup: given an existing rule's WhenPredicate, find which
// preset token (if any) produced it, so the editor can select the right option when
// loading an existing rule. A predicate that isn't a byte-for-byte match to a known
// preset (e.g. hand-authored via the API, or a composite) falls back to "" (shown as
// "Custom" in the select — still saved as-is since the editor only ever REPLACES the
// token's own fields, never the whole rule from scratch, when a preset can't be matched).
const tokenForWhen = (w: WhenPredicate | undefined): string => {
  if (!w) return "";
  const match = WHEN_PRESETS.find(
    (p) =>
      Boolean(p.predicate.weekend) === Boolean(w.weekend) &&
      Boolean(p.predicate.weekday) === Boolean(w.weekday) &&
      (p.predicate.hourFrom ?? 0) === (w.hourFrom ?? 0) &&
      (p.predicate.hourTo ?? 0) === (w.hourTo ?? 0) &&
      (p.predicate.holiday ?? "") === (w.holiday ?? ""),
  );
  return match?.token ?? "";
};

// ---- WHAT tokens (ScopePolicy) -------------------------------------------------------

type WhatKind = "all" | "series" | "genre" | "kids" | "family" | "holiday-matched" | "era";

interface WhatOption {
  value: string;
  label: string;
}

// Static WHAT options (the ones that don't need the channel's lineup). `series:<key>` is
// appended by the caller from `lineupKeys` (§6.6: "intersected with the channel's
// actually-grounded picks").
const WHAT_STATIC_OPTIONS: WhatOption[] = [
  { value: "all", label: "Anything (no extra narrowing)" },
  { value: "kids", label: "Kids-safe genres" },
  { value: "family", label: "Family genres" },
  { value: "holiday-matched", label: "Holiday-matched titles" },
];

// lowerWhat — the FE mirror of LowerWhat (presets.go). Returns undefined scope for "all"
// (no narrowing) and a ScopePolicy for a recognized narrowing token. Unknown → null (drop).
const lowerWhat = (token: string): { scope: ScopePolicy | undefined } | undefined => {
  const t = token.trim();
  if (t === "" || t === "all") return { scope: undefined };
  if (t === "kids") return { scope: { genres: { include: ["Animation", "Family", "Kids"] } } };
  if (t === "family")
    return { scope: { genres: { include: ["Family", "Animation", "Adventure", "Comedy"] } } };
  if (t === "holiday-matched") return { scope: undefined }; // seasonal engine handles the filter, not scope
  if (t.startsWith("series:")) {
    const key = t.slice("series:".length).trim();
    if (!key) return undefined;
    return { scope: { series: [key] } };
  }
  if (t.startsWith("genre:")) {
    const g = t.slice("genre:".length).trim();
    if (!g) return undefined;
    return { scope: { genres: { include: [g] } } };
  }
  if (t.startsWith("era:")) {
    const parsed = parseEraToken(t.slice("era:".length));
    if (!parsed) return undefined;
    return { scope: { era: parsed } };
  }
  return undefined;
};

// tokenForWhat — the reverse lookup for an existing rule's ScopePolicy, so the editor can
// preselect the WHAT option when loading a rule. Falls back to "all" for a nil/empty
// scope and "" ("Custom") for a scope shape none of the presets produce.
const tokenForWhat = (scope: ScopePolicy | null | undefined): string => {
  if (!scope) return "all";
  const series = scope.series;
  if (series && series.length > 0) return `series:${series[0]}`;
  const include = scope.genres?.include ?? [];
  if (arraysEqual(include, ["Animation", "Family", "Kids"])) return "kids";
  if (arraysEqual(include, ["Family", "Animation", "Adventure", "Comedy"])) return "family";
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

// parseEraToken — mirrors parseEraToken (presets.go): "1990-1999" / "1990-" / "-1999".
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

// ---- HOW tokens (RuleOrdering + Window) ----------------------------------------------

interface HowPreset {
  token: string;
  label: string;
  // A terser form of `label` for the computed row caption (label.ts) — the Select's own
  // item text carries the parenthetical explanation, so the caption doesn't repeat it.
  shortLabel: string;
  ordering: RuleOrdering;
  // "full" = the WindowFull sentinel (marathon: don't truncate a binge); undefined =
  // inherit the channel/global window.
  window?: "full";
}

// WindowFull sentinel — mirrors schedule.WindowFull (-1) on the Go side. The wire type
// for ChannelPolicy/SchedulingRule.window is a Go-duration STRING; there is no numeric
// "-1" on this side of the wire, so the editor tracks "full run" as a distinct sentinel
// string and the caller (channel-rules-editor.tsx) is responsible for putting it on
// `rule.window` in whatever string form the rest of the app already uses for "unbounded"
// (kept out of this pure lowering table, which only knows tokens ⇄ RuleOrdering).
const HOW_PRESETS: HowPreset[] = [
  {
    token: "syndication",
    label: "Syndication (the deck order)",
    shortLabel: "Syndication",
    ordering: { ordering: "syndication" },
  },
  { token: "shuffle", label: "Shuffle", shortLabel: "Shuffle", ordering: { ordering: "shuffle" } },
  {
    token: "marathon",
    label: "Marathon (binge one show, no breaks)",
    shortLabel: "Marathon",
    ordering: { ordering: "sequential", noBreaks: true, separation: { blockMax: -1 } },
    window: "full",
  },
  {
    token: "feature",
    label: "Feature (in order)",
    shortLabel: "Feature",
    ordering: { ordering: "sequential" },
  },
];

const HOW_OPTIONS: { value: string; label: string }[] = HOW_PRESETS.map((p) => ({
  value: p.token,
  label: p.label,
}));

// HOW_SHORT_LABELS — token → terse label, for the computed row caption (label.ts).
const HOW_SHORT_LABELS: { value: string; label: string }[] = HOW_PRESETS.map((p) => ({
  value: p.token,
  label: p.shortLabel,
}));

// lowerHow — the FE mirror of LowerHow (presets.go). "sequential" is accepted as an
// alias for "feature" on the Go side but the editor only ever emits the canonical token.
const lowerHow = (token: string): { ordering: RuleOrdering; window?: "full" } | undefined => {
  const preset = HOW_PRESETS.find((p) => p.token === token);
  if (!preset) return undefined;
  return { ordering: preset.ordering, window: preset.window };
};

// tokenForHow — the reverse lookup for an existing rule's RuleOrdering, so the editor can
// preselect the HOW option when loading a rule. "" ("Custom") when nothing matches.
const tokenForHow = (how: RuleOrdering | undefined): string => {
  if (!how?.ordering) return "";
  const match = HOW_PRESETS.find(
    (p) =>
      p.ordering.ordering === how.ordering &&
      Boolean(p.ordering.noBreaks) === Boolean(how.noBreaks) &&
      (p.ordering.separation?.blockMax ?? 0) === (how.separation?.blockMax ?? 0),
  );
  return match?.token ?? "";
};

export type { WhatKind };
export {
  HOLIDAY_IDS,
  HOW_OPTIONS,
  HOW_SHORT_LABELS,
  lowerHow,
  lowerWhat,
  lowerWhen,
  tokenForHow,
  tokenForWhat,
  tokenForWhen,
  WHAT_STATIC_OPTIONS,
  WHEN_OPTIONS,
  WHEN_SHORT_LABELS,
};
