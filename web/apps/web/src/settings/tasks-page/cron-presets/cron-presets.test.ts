import { describe, expect, it } from "vitest";
import { CRON_PRESETS, CUSTOM_VALUE, describeCron, isPreset } from "./cron-presets";

describe("cron-presets", () => {
  it("labels a known preset by its friendly name", () => {
    expect(describeCron("0 */5 * * * *")).toBe("Every 5 minutes");
    expect(describeCron("0 0 3 * * *")).toBe("Daily at 3 am");
  });

  it("trims whitespace before matching", () => {
    expect(describeCron("  0 */5 * * * *  ")).toBe("Every 5 minutes");
    expect(isPreset("  0 0 * * * *  ")).toBe(true);
  });

  it("returns Custom for an expression that matches no preset", () => {
    expect(describeCron("0 30 4 * * 1")).toBe("Custom");
    expect(isPreset("0 30 4 * * 1")).toBe(false);
  });

  it("keeps the sentinel distinct from any real cron", () => {
    // CUSTOM_VALUE drives the "Advanced" dropdown option; it must never collide with a preset.
    expect(CRON_PRESETS.some((p) => p.cron === CUSTOM_VALUE)).toBe(false);
  });

  it("every preset cron is a distinct 6-field seconds-leading expression", () => {
    const crons = CRON_PRESETS.map((p) => p.cron);
    expect(new Set(crons).size).toBe(crons.length);
    for (const c of crons) {
      expect(c.split(" ")).toHaveLength(6);
    }
  });
});
