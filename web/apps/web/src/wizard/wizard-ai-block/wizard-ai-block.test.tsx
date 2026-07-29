import type { SettingEntry } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WizardAiBlock } from "./wizard-ai-block";

const json = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// The `ai` settings group as the registry ships it: the provider dropdown plus the hosted
// URL/key fields that reveal themselves (ShowWhen) when provider = openai (config-design §5).
const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key">): SettingEntry => ({
  group: "ai",
  kind: "string",
  advanced: false,
  secret: false,
  set: false,
  doc: "",
  provenance: "default",
  ...over,
});
const AI_ENTRIES: SettingEntry[] = [
  entry({
    key: "llm.provider",
    kind: "enum",
    value: "ollama",
    enumOptions: [
      { value: "ollama", label: "Ollama" },
      { value: "openai", label: "OpenAI-compatible" },
    ],
  }),
  entry({ key: "llm.url", kind: "url", showWhen: { "llm.provider": ["openai"] } }),
  entry({ key: "llm.api_key", kind: "secret", secret: true, showWhen: { "llm.provider": ["openai"] } }),
];

// A fetch stub that returns the AI settings group + an Ollama probe result. `over` tweaks the
// probe (reachable / model) per test.
const stubFetch = (probe: Record<string, unknown>) =>
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      const u = String(url);
      if (u.includes("/v1/system/llm")) {
        return Promise.resolve(json({ provider: "ollama", catalog: [], hosted: [], ...probe }));
      }
      if (u.includes("/v1/settings")) return Promise.resolve(json({ settings: AI_ENTRIES, features: {} }));
      return Promise.resolve(json({}));
    }),
  );

const renderBlock = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <WizardAiBlock />
    </QueryClientProvider>,
  );
};

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("WizardAiBlock", () => {
  it("confirms the detected Ollama model when one is active", async () => {
    stubFetch({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    await waitFor(() => expect(screen.getByText(/Ollama detected: using qwen3:8b/i)).toBeInTheDocument());
  });

  it("shows a neutral 'no local Ollama' hint (never a red error) when unreachable", async () => {
    stubFetch({ reachable: false, model: "" });
    renderBlock();
    await waitFor(() => expect(screen.getByText(/No local Ollama found/i)).toBeInTheDocument());
    expect(screen.getByText(/suggestions stay off until you do/i)).toBeInTheDocument();
  });

  it("offers the provider choice — the LLM provider control is present", async () => {
    stubFetch({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    // The provider dropdown from the ai settings group renders (humanized "LLM provider").
    // It's a combobox (Radix), so we assert its presence rather than drive its options in
    // jsdom — the switch-reveals-hosted-fields behaviour is verified live in the browser.
    await waitFor(() => expect(screen.getByText("LLM provider")).toBeInTheDocument());
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("hides the hosted URL/key fields while the provider is Ollama (ShowWhen)", async () => {
    stubFetch({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    await waitFor(() => expect(screen.getByText("LLM provider")).toBeInTheDocument());
    // provider=ollama → the openai-only URL + key fields are not shown.
    expect(screen.queryByText(/LLM url/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/LLM api key/i)).not.toBeInTheDocument();
  });
});
