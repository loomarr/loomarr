import { describe, expect, it } from "vitest";
import { parseFillerSettingsSection, SETTINGS_SECTIONS } from "./filler-settings-section";

describe("Filler settings task links", () => {
  it.each(SETTINGS_SECTIONS)("accepts the $id task", ({ id }) => {
    expect(parseFillerSettingsSection(id)).toBe(id);
  });

  it.each([undefined, "unknown", [], 4, { id: "storage" }])("rejects invalid path input %j", (section) => {
    expect(parseFillerSettingsSection(section)).toBeUndefined();
  });
});
