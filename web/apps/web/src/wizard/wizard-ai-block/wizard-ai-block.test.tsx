import type { SettingEntry, SystemLLMStatus } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import {
  getSettingsListMockHandler,
  getSystemLlmDiscoverMockHandler,
  getSystemLlmStatusMockHandler,
  getSystemLlmTestMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { WizardAiBlock } from "./wizard-ai-block";

// The `ai` settings group as the registry ships it: the provider dropdown plus the hosted
// URL/key fields that reveal themselves (ShowWhen) when provider = openai (config-design §5).
const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key">): SettingEntry => ({
  group: "ai",
  kind: "string",
  advanced: false,
  apply: SettingEntryApply.live,
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

// The AI settings group plus an Ollama probe result; `probe` tweaks reachability/model per test.
//
// ⚠ The stub this replaced ended in `return Promise.resolve(json({}))` — a catch-all that answered
// any OTHER url with an empty object. That is precisely what makes a hand-rolled stub unable to
// fail: an unexpected request was indistinguishable from an expected one. Only the three endpoints
// this block actually uses are stubbed now, and anything else fails the test by name.
// ⚠ `probe` is typed as the two fields the tests vary, NOT `Record<string, unknown>`. The loose
// version type-checked only because a hand-rolled stub never had to satisfy SystemLLMStatus —
// which requires `local`, `model`, `reachable`, `catalog`, `hosted` and `provider`, and the old
// fixture supplied neither `local` nor a typed `reachable`.
const stubAi = (probe: Pick<SystemLLMStatus, "reachable" | "model">) =>
  server.use(
    // ⚠ `local` and `model` are REQUIRED on SystemLLMStatus and the old stub supplied neither —
    // another incomplete fixture an untyped stub let through.
    getSystemLlmStatusMockHandler({ provider: "ollama", local: true, catalog: [], hosted: [], ...probe }),
    getSettingsListMockHandler({ settings: AI_ENTRIES, features: {} }),
    // ⚠ The block also calls /v1/system/llm/discover — the OLD stub answered it with `{}` from its
    // catch-all, so this code path ran against an empty object and no one knew. The guard named it.
    getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
  );

const renderBlock = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <WizardAiBlock />
    </QueryClientProvider>,
  );
};

describe("WizardAiBlock", () => {
  it("confirms the detected Ollama model when one is active", async () => {
    stubAi({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    await waitFor(() => expect(screen.getByText(/Ollama detected: using qwen3:8b/i)).toBeInTheDocument());
  });

  it("shows a neutral 'no local Ollama' hint (never a red error) when unreachable", async () => {
    stubAi({ reachable: false, model: "" });
    renderBlock();
    await waitFor(() => expect(screen.getByText(/No local Ollama found/i)).toBeInTheDocument());
    expect(screen.getByText(/suggestions stay off until you do/i)).toBeInTheDocument();
  });

  it("offers the provider choice — the LLM provider control is present", async () => {
    stubAi({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    // The provider dropdown from the ai settings group renders (humanized "LLM provider").
    // ⚠ It is a Base UI combobox — this comment said "Radix" until V53e, which stopped being true
    // at V50a when the vendor moved. The reasoning is unchanged and was never Radix-specific: its
    // options are asserted for PRESENCE rather than driven in jsdom, because the
    // switch-reveals-hosted-fields behaviour is verified live in the browser.
    await waitFor(() => expect(screen.getByText("LLM provider")).toBeInTheDocument());
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("hides the hosted URL/key fields while the provider is Ollama (ShowWhen)", async () => {
    stubAi({ reachable: true, model: "qwen3:8b" });
    renderBlock();
    await waitFor(() => expect(screen.getByText("LLM provider")).toBeInTheDocument());
    // provider=ollama → the openai-only URL + key fields are not shown.
    expect(screen.queryByText(/LLM url/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/LLM api key/i)).not.toBeInTheDocument();
  });

  it("reports valid hosted credentials separately from missing model readiness", async () => {
    const requests: unknown[] = [];
    server.use(
      getSystemLlmStatusMockHandler({
        provider: "openrouter",
        local: false,
        reachable: true,
        model: "",
        catalog: [],
        hosted: [
          {
            key: "openrouter",
            label: "OpenRouter",
            baseUrl: "https://openrouter.ai/api/v1",
            keysUrl: "https://openrouter.ai/keys",
            keyConfigured: true,
            active: true,
            modelsLive: true,
            models: [{ id: "openai/gpt-4o-mini", label: "GPT-4o mini", tools: true }],
          },
        ],
      }),
      getSettingsListMockHandler({
        settings: AI_ENTRIES.map((candidate) =>
          candidate.key === "llm.provider" ? { ...candidate, value: "openai" } : candidate,
        ),
        features: {},
      }),
      getSystemLlmTestMockHandler(async ({ request }) => {
        requests.push(await request.json());
        return { ok: true };
      }),
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
    );

    renderBlock();
    await userEvent.click(await screen.findByRole("button", { name: "Test provider credentials" }));

    expect(await screen.findByRole("status")).toHaveTextContent(
      "OpenRouter credentials authorized. Choose a tool-capable lineup model to finish AI setup.",
    );
    expect(requests).toEqual([{ provider: "openrouter" }]);
  });
});
