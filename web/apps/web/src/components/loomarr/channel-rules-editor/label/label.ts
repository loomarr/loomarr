import { HOW_SHORT_LABELS, WHAT_STATIC_OPTIONS, WHEN_SHORT_LABELS } from "../presets";

// computeLabel — the FE mirror of SchedulingRule.Describe() (internal/schedule/rule.go).
// The Go side synthesizes a label from When/How only when Label is empty (What doesn't
// contribute — Describe() never reads it either, since a scope alone doesn't read as a
// standalone phrase the way a time/ordering phrase does). Kept as its own small module
// (rather than folded into presets.ts) because it's presentation, not token→field
// lowering — a label is cosmetic and the engine never keys behavior on it (rule.go's own
// comment on SchedulingRule.Label).
//
// This FE version additionally weaves in the WHAT token's label when present and not
// "all", since the editor shows what/when/how as three independent pickers and a user
// benefits from seeing all three named, not just when/how — joined with " · " to read as
// a compact row caption rather than the BE's " — " prose join. Uses each table's SHORT
// labels (no parenthetical explanation) since that detail already lives on the Select's
// own item text — the caption stays terse ("Weekend · Marathon", not "Weekend · Marathon
// (binge one show, no breaks)").
const whenLabel = (token: string): string => WHEN_SHORT_LABELS.find((o) => o.value === token)?.label ?? "";
const whatLabel = (token: string): string => WHAT_STATIC_OPTIONS.find((o) => o.value === token)?.label ?? "";
const howLabel = (token: string): string => HOW_SHORT_LABELS.find((o) => o.value === token)?.label ?? "";

// computeLabel joins the non-empty token labels with " · " (e.g. "Weekend · Marathon").
// A `series:<key>`/`genre:<name>`/`era:<range>` WHAT token isn't in the static label
// table (it carries a caller-supplied title), so the caller passes its display text
// directly via `whatDisplay` when it isn't one of the fixed WHAT_STATIC_OPTIONS.
const computeLabel = (params: {
  whenToken: string;
  whatToken: string;
  whatDisplay?: string;
  howToken: string;
}): string => {
  const { whenToken, whatToken, whatDisplay, howToken } = params;
  const parts = [
    whenLabel(whenToken),
    whatToken === "all" || whatToken === "" ? "" : (whatDisplay ?? whatLabel(whatToken)),
    howLabel(howToken),
  ].filter((p) => p.length > 0);
  return parts.length > 0 ? parts.join(" · ") : "Rule";
};

export { computeLabel };
