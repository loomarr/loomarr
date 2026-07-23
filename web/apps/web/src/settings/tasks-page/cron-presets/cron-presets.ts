// Human-readable schedule presets for the "Modify Job" modal (§18.1). Each maps a friendly
// label to a canonical 6-field seconds-leading cron (matching the BE / Overseerr format). The
// modal offers these in a dropdown by default; an "Advanced" toggle reveals the raw cron for
// power users. A saved cron not matching any preset shows as "Custom" with its raw expression.

interface CronPreset {
  label: string;
  cron: string;
}

// Ordered from most to least frequent — the common household choices.
const CRON_PRESETS: CronPreset[] = [
  { label: "Every minute", cron: "0 * * * * *" },
  { label: "Every 5 minutes", cron: "0 */5 * * * *" },
  { label: "Every 10 minutes", cron: "0 */10 * * * *" },
  { label: "Every 15 minutes", cron: "0 */15 * * * *" },
  { label: "Every 30 minutes", cron: "0 */30 * * * *" },
  { label: "Every hour", cron: "0 0 * * * *" },
  { label: "Every 6 hours", cron: "0 0 */6 * * *" },
  { label: "Every 12 hours", cron: "0 0 */12 * * *" },
  { label: "Daily at 3 am", cron: "0 0 3 * * *" },
  { label: "Daily at midnight", cron: "0 0 0 * * *" },
];

const CRON_TO_LABEL = new Map(CRON_PRESETS.map((p) => [p.cron, p.label]));

// A stable sentinel value for the "Custom (advanced cron)" dropdown option — not a real cron.
const CUSTOM_VALUE = "__custom__";

// describeCron returns a human label for a cron string: a preset's label when it matches one,
// else "Custom" (the raw expression is shown separately). Used in the table + the modal.
function describeCron(cron: string): string {
  return CRON_TO_LABEL.get(cron.trim()) ?? "Custom";
}

// isPreset reports whether a cron string exactly matches one of the presets.
function isPreset(cron: string): boolean {
  return CRON_TO_LABEL.has(cron.trim());
}

export type { CronPreset };
export { CRON_PRESETS, CUSTOM_VALUE, describeCron, isPreset };
