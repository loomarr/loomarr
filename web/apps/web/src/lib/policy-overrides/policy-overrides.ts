import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { DateScope } from "@loomarr/api/models/dateScope";
import { eraOf } from "@/lib/era-dates";

// The Programming tab's defaultable fields (#1817 item 4). Each one is either a channel override
// or the sentinel `schedule.ChannelPolicy` reads as "inherit" (internal/schedule/policy.go). Most
// of those inherited values are Loomarr's built-ins, not a household setting, which is why the
// summary says so rather than pointing at Settings.
//
// The lineup, rules, series and collections scopes are authored content, not defaults, so they
// are not fields here.
const POLICY_FIELDS = [
  "ceiling",
  "unrated",
  "dates",
  "runtimeMax",
  "ordering",
  "movieNoRepeat",
  "episodeNoRepeat",
  "seriesMinGap",
  "blockMax",
  "seasonalMode",
  "autoCurate",
] as const;

type PolicyField = (typeof POLICY_FIELDS)[number];

// Go's Duration.String() spells zero "0s"; omitempty drops it, so both mean built-in spacing.
const isZeroDuration = (v: string | undefined): boolean => !v || v === "0s";

const hasDates = (dates: DateScope | undefined): boolean =>
  Object.values(dates ?? {}).some((ranges) => Array.isArray(ranges) && ranges.length > 0);

// isOverridden — whether a field holds a channel value rather than its inherit sentinel.
const isOverridden = (policy: ChannelPolicy, field: PolicyField): boolean => {
  switch (field) {
    case "ceiling":
      return Boolean(policy.audience?.ceiling);
    case "unrated":
      return Boolean(policy.audience?.unrated);
    case "dates":
      return hasDates(policy.scope?.dates);
    case "runtimeMax":
      return (policy.scope?.runtimeMax ?? 0) > 0;
    case "ordering":
      return Boolean(policy.ordering);
    case "movieNoRepeat":
      return !isZeroDuration(policy.separation?.movieNoRepeat);
    case "episodeNoRepeat":
      return !isZeroDuration(policy.separation?.episodeNoRepeat);
    case "seriesMinGap":
      return !isZeroDuration(policy.separation?.seriesMinGap);
    case "blockMax":
      // omitempty int: 0 is the built-in cap, the same as absent.
      return (policy.separation?.blockMax ?? 0) !== 0;
    case "seasonalMode":
      // "" and the explicit "auto" resolve identically (SeasonalPolicy.Mode).
      return Boolean(policy.seasonal?.mode) && policy.seasonal?.mode !== "auto";
    case "autoCurate":
      // The opt-in IS the object's presence (*AutoCurate).
      return policy.autoCurate !== undefined;
  }
};

// resetField — the policy with one field back at its sentinel. The sentinels match the field
// editor's own "cleared" values: "" for the string enums, 0 for runtimeMax (a cleared box must
// send 0), undefined for separation (0 would mean something else for a duration), and an absent
// key for dates and the auto-curate opt-in.
const resetField = (policy: ChannelPolicy, field: PolicyField): ChannelPolicy => {
  switch (field) {
    case "ceiling":
      return { ...policy, audience: { ...policy.audience, ceiling: "" } };
    case "unrated":
      return { ...policy, audience: { ...policy.audience, unrated: "" } };
    case "dates": {
      const { dates: _dates, ...scope } = policy.scope ?? {};
      return { ...policy, scope };
    }
    case "runtimeMax":
      return { ...policy, scope: { ...policy.scope, runtimeMax: 0 } };
    case "ordering":
      return { ...policy, ordering: "" };
    case "movieNoRepeat":
    case "episodeNoRepeat":
    case "seriesMinGap":
    case "blockMax":
      return { ...policy, separation: { ...policy.separation, [field]: undefined } };
    case "seasonalMode":
      return { ...policy, seasonal: { ...policy.seasonal, mode: "" } };
    case "autoCurate": {
      const { autoCurate: _autoCurate, ...rest } = policy;
      return rest;
    }
  }
};

// overrideCount — the summary's split count: how many of the page's defaultable fields this
// channel overrides.
const overrideCount = (policy: ChannelPolicy): { overridden: number; total: number } => ({
  overridden: POLICY_FIELDS.filter((field) => isOverridden(policy, field)).length,
  total: POLICY_FIELDS.length,
});

// datesMode — which editor the dates field shows. Era-shaped dates (the same single range on
// all three axes) and no dates at all are the Era simple mode; anything else needs the per-axis
// editor, because an era cannot express it.
const datesMode = (dates: DateScope | undefined): "era" | "axes" =>
  !hasDates(dates) || eraOf(dates) ? "era" : "axes";

// The two auto-curate thresholds refine the opt-in rather than standing alone, so they are not
// in the count. Both are `0 = inherit the global default` (int64, omitempty).
type AutoCurateThreshold = "minScorePct" | "maxTitles";

const isThresholdOverridden = (policy: ChannelPolicy, threshold: AutoCurateThreshold): boolean =>
  (policy.autoCurate?.[threshold] ?? 0) > 0;

const resetThreshold = (policy: ChannelPolicy, threshold: AutoCurateThreshold): ChannelPolicy => ({
  ...policy,
  autoCurate: { ...policy.autoCurate, [threshold]: 0 },
});

export type { AutoCurateThreshold, PolicyField };
export {
  datesMode,
  isOverridden,
  isThresholdOverridden,
  overrideCount,
  POLICY_FIELDS,
  resetField,
  resetThreshold,
};
