import { SettingKey } from "@loomarr/api/models/settingKey";
import { describe, expect, it } from "vitest";
import { REUSE_LINEUP, sameVisionChoice, visionChoiceOf, visionChoiceSettings } from "./vision-choice";

describe("visionChoiceOf", () => {
  it("reads inherit with no model as reusing the lineup model", () => {
    expect(visionChoiceOf("inherit", "", true)).toEqual(REUSE_LINEUP);
    expect(visionChoiceOf("", "", false)).toEqual(REUSE_LINEUP);
  });

  it("reads a local Ollama model whatever the lineup uses", () => {
    expect(visionChoiceOf("ollama", "llava:7b", true)).toEqual({ provider: "ollama", model: "llava:7b" });
  });

  it("reads inherit with a model as a model on the hosted lineup's provider", () => {
    expect(visionChoiceOf("inherit", "openai/gpt-4o", true)).toEqual({
      provider: "inherit",
      model: "openai/gpt-4o",
    });
  });

  // Under a local lineup the lineup's provider is Ollama, so the stored inherit pair is the
  // same choice as the Ollama option the picker shows for it.
  it("reads inherit with a model under a local lineup as that Ollama model", () => {
    expect(visionChoiceOf("inherit", "llava:7b", false)).toEqual({ provider: "ollama", model: "llava:7b" });
  });
});

describe("visionChoiceSettings", () => {
  it("writes both settings, so a choice never leaves the other half behind", () => {
    expect(visionChoiceSettings(REUSE_LINEUP)).toEqual([
      [SettingKey.FillerVisionProvider, "inherit"],
      [SettingKey.FillerVisionModel, ""],
    ]);
    expect(visionChoiceSettings({ provider: "ollama", model: "llava:7b" })).toEqual([
      [SettingKey.FillerVisionProvider, "ollama"],
      [SettingKey.FillerVisionModel, "llava:7b"],
    ]);
  });
});

describe("sameVisionChoice", () => {
  it("compares provider and model", () => {
    expect(sameVisionChoice(REUSE_LINEUP, { provider: "inherit", model: "" })).toBe(true);
    expect(sameVisionChoice({ provider: "ollama", model: "a" }, { provider: "inherit", model: "a" })).toBe(
      false,
    );
  });
});
