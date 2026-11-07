import type { SettingPreset } from "@loomarr/api/models/settingPreset";

// Human-readable schedules for the Tasks page and its "Modify Job" dialog (§18.1). The choices
// are the registry's presets on the job's schedule setting; this module only matches a cron
// against them. A saved cron matching none shows as "Custom" with its raw expression.

// A stable sentinel value for the "Custom (advanced cron)" dropdown option — not a real cron.
const CUSTOM_VALUE = "__custom__";

const matching = (cron: string, presets: readonly SettingPreset[]) =>
  presets.find((preset) => preset.value === cron.trim());

// describeCron returns a human label for a cron string: the matching preset's label, else
// "Custom" (the raw expression is shown separately).
function describeCron(cron: string, presets: readonly SettingPreset[]): string {
  return matching(cron, presets)?.label ?? "Custom";
}

// isPreset reports whether a cron string exactly matches one of the presets.
function isPreset(cron: string, presets: readonly SettingPreset[]): boolean {
  return matching(cron, presets) !== undefined;
}

export { CUSTOM_VALUE, describeCron, isPreset };
