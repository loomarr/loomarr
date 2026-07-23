import type { SettingEntry } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { blockTitle, providerSuffix } from "./provider-title";

const entry = (key: string, value: string): SettingEntry => ({ key, value }) as SettingEntry;

describe("provider-title", () => {
  it("suffixes the saved requester provider", () => {
    expect(blockTitle([entry("requester.provider", "seerr")], "Requester", "requester.provider")).toBe(
      "Requester (Seerr)",
    );
    expect(blockTitle([entry("requester.provider", "arr")], "Requester", "requester.provider")).toBe(
      "Requester (Sonarr + Radarr)",
    );
  });

  it("suffixes the saved AI provider", () => {
    expect(blockTitle([entry("llm.provider", "ollama")], "AI", "llm.provider")).toBe("AI (Ollama)");
    expect(blockTitle([entry("llm.provider", "openai")], "AI", "llm.provider")).toBe(
      "AI (OpenAI-compatible)",
    );
  });

  it("adds no suffix when the provider is unset or unknown", () => {
    // No entry → no suffix (nothing chosen + saved yet).
    expect(providerSuffix([], "requester.provider")).toBe("");
    // An unrecognized value → no suffix rather than a misleading "(foo)".
    expect(providerSuffix([entry("requester.provider", "wat")], "requester.provider")).toBe("");
  });
});
