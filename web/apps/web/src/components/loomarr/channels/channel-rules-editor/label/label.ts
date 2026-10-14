import type { Vocabulary } from "@loomarr/api/models/vocabulary";
import { howShortLabels, whatStaticOptions, whenShortLabels } from "../presets";

// computeLabel — the FE mirror of SchedulingRule.Describe() (internal/schedule/rule.go).
// The Go side synthesizes a label from When/How only when Label is empty; this FE version
// additionally weaves in the WHAT token's label (except "all") so the editor's row caption
// names all three pickers, joined with " · " to read as a compact caption. Presentation
// only — the engine never keys behavior on the label (rule.go's own comment). Labels come
// from the served vocabulary (§6.6), so a token's caption reads the same as its picker text.
const computeLabel = (
  params: { whenToken: string; whatToken: string; whatDisplay?: string; howToken: string },
  vocab: Vocabulary,
): string => {
  const { whenToken, whatToken, whatDisplay, howToken } = params;
  const whenLabel = whenShortLabels(vocab.when).find((o) => o.value === whenToken)?.label ?? "";
  const whatLabel = whatStaticOptions(vocab.what).find((o) => o.value === whatToken)?.label ?? "";
  const howLabel = howShortLabels(vocab.how).find((o) => o.value === howToken)?.label ?? "";
  const parts = [
    whenLabel,
    whatToken === "all" || whatToken === "" ? "" : (whatDisplay ?? whatLabel),
    howLabel,
  ].filter((p) => p.length > 0);
  return parts.length > 0 ? parts.join(" · ") : "Rule";
};

export { computeLabel };
