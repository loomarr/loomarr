import type { SettingEntry } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SettingsFields } from "./settings-fields";

const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key" | "kind">): SettingEntry => ({
  group: "ai",
  owner: "settings.ai",
  doc: "help",
  advanced: false,
  apply: SettingEntryApply.live,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

// The AI group: provider (enum) controls whether url + key show (config-design §5 ShowWhen).
const aiEntries = (): SettingEntry[] => [
  entry({
    key: "llm.provider",
    kind: "enum",
    value: "ollama",
    enum: ["ollama", "openai"],
    enumOptions: [
      { value: "ollama", label: "Ollama" },
      { value: "openai", label: "OpenAI-compatible" },
    ],
  }),
  entry({ key: "llm.url", kind: "url", value: "", showWhen: { "llm.provider": ["openai"] } }),
  entry({ key: "llm.model", kind: "string", value: "" }),
  entry({
    key: "llm.api_key",
    kind: "secret",
    secret: true,
    value: "",
    showWhen: { "llm.provider": ["openai"] },
  }),
];

describe("SettingsFields — conditional fields (ShowWhen)", () => {
  it("hides url + key for Ollama (they only apply to a hosted service)", () => {
    render(<SettingsFields entries={aiEntries()} values={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText("LLM provider")).toBeInTheDocument();
    expect(screen.getByLabelText("LLM model")).toBeInTheDocument();
    expect(screen.queryByLabelText("LLM URL")).not.toBeInTheDocument();
    // The secret's Replace affordance is absent too (the whole field is hidden).
    expect(screen.queryByText(/api key/i)).not.toBeInTheDocument();
  });

  it("shows url + key when the stored provider is openai", () => {
    const entries = aiEntries().map((e) => (e.key === "llm.provider" ? { ...e, value: "openai" } : e));
    render(<SettingsFields entries={entries} values={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText("LLM URL")).toBeInTheDocument();
  });

  it("reveals url + key live when the provider edit switches to openai (before saving)", async () => {
    // An unsaved edit in `values` must drive visibility, not just the stored value.
    const { rerender } = render(<SettingsFields entries={aiEntries()} values={{}} onChange={vi.fn()} />);
    expect(screen.queryByLabelText("LLM URL")).not.toBeInTheDocument();

    // Simulate the parent recording the provider edit.
    rerender(
      <SettingsFields entries={aiEntries()} values={{ "llm.provider": "openai" }} onChange={vi.fn()} />,
    );
    expect(screen.getByLabelText("LLM URL")).toBeInTheDocument();
  });

  it("a field with no ShowWhen is always visible", () => {
    render(<SettingsFields entries={aiEntries()} values={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText("LLM model")).toBeInTheDocument();
  });
});
