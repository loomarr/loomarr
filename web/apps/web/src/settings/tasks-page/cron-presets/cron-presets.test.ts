import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { describe, expect, it } from "vitest";
import { CUSTOM_VALUE, describeCron, isPreset } from "./cron-presets";

// The job schedule's presets as the registry publishes them on its setting entry.
const presets: SettingPreset[] = [
  { value: "0 */5 * * * *", label: "Every 5 minutes" },
  { value: "0 0 3 * * *", label: "Daily at 3 am" },
];

describe("cron-presets", () => {
  it("labels a schedule by the matching preset", () => {
    expect(describeCron("0 */5 * * * *", presets)).toBe("Every 5 minutes");
    expect(describeCron("0 0 3 * * *", presets)).toBe("Daily at 3 am");
  });

  it("trims whitespace before matching", () => {
    expect(describeCron("  0 */5 * * * *  ", presets)).toBe("Every 5 minutes");
    expect(isPreset("  0 0 3 * * *  ", presets)).toBe(true);
  });

  it("returns Custom for an expression that matches no preset", () => {
    expect(describeCron("0 30 4 * * 1", presets)).toBe("Custom");
    expect(isPreset("0 30 4 * * 1", presets)).toBe(false);
  });

  it("returns Custom when the setting offers no presets", () => {
    expect(describeCron("0 */5 * * * *", [])).toBe("Custom");
  });

  it("keeps the sentinel distinct from any cron", () => {
    // A cron has six space-separated fields; the sentinel has none.
    expect(CUSTOM_VALUE.split(" ")).toHaveLength(1);
  });
});
