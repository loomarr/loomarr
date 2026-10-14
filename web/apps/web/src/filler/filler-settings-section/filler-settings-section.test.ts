import { describe, expect, it } from "vitest";
import { parseFillerSettingsSection, SETTINGS_SECTIONS } from "./filler-settings-section";

describe("Filler settings task links", () => {
  it.each(SETTINGS_SECTIONS)("accepts the $id task", ({ id }) => {
    expect(parseFillerSettingsSection(id)).toBe(id);
  });

  it.each([undefined, "unknown", [], 4, { id: "storage" }])("rejects invalid path input %j", (section) => {
    expect(parseFillerSettingsSection(section)).toBeUndefined();
  });

  it("keeps every task visible in one index group with useful copy", () => {
    expect(SETTINGS_SECTIONS.filter((item) => item.group === "everyday").map((item) => item.id)).toEqual([
      "downloads",
      "folders",
      "storage",
      "details",
      "incoming",
      "breaks",
      "review",
      "playback",
    ]);
    expect(SETTINGS_SECTIONS.filter((item) => item.group === "advanced").map((item) => item.id)).toEqual([
      "limits",
      "tools",
    ]);
    for (const item of SETTINGS_SECTIONS) {
      expect(item.description).not.toBe("");
      expect(item.aliases.length).toBeGreaterThan(0);
    }
  });
});
