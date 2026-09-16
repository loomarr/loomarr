import { describe, expect, it } from "vitest";
import { SETTINGS_SECTIONS, validateFillerSettingsSearch } from "./filler-settings-search";

describe("Filler settings task links", () => {
  it.each(SETTINGS_SECTIONS)("accepts the $id task", ({ id }) => {
    expect(validateFillerSettingsSearch({ section: id })).toEqual({ section: id });
  });

  it.each([undefined, "unknown", [], 4, { id: "storage" }])(
    "defaults invalid input %j to downloads",
    (section) => {
      expect(validateFillerSettingsSearch({ section })).toEqual({ section: "downloads" });
    },
  );
});
