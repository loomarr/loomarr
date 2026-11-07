import type { LLMModelView } from "@loomarr/api/models/lLMModelView";
import { SettingKey } from "@loomarr/api/models/settingKey";
import type { SystemLLMStatus } from "@loomarr/api/models/systemLLMStatus";
import {
  getSystemLlmDiscoverMockHandler,
  getSystemLlmStatusMockHandler,
  getSystemLlmTestMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { RouterHarness } from "@/test/story-utils";
import { AiModelSettings } from "./ai-model-settings";

const localModel = (over: Partial<LLMModelView>): LLMModelView => ({
  tag: "qwen3:8b",
  label: "Qwen3 8B",
  approxVramGiB: 5,
  fit: "fits",
  pulled: true,
  recommended: false,
  runtimeOk: true,
  tools: true,
  vision: false,
  why: "Good fit.",
  ...over,
});

const ollama = (over: Partial<SystemLLMStatus> = {}): SystemLLMStatus => ({
  local: true,
  reachable: true,
  provider: "ollama",
  model: "qwen3:8b",
  catalog: [localModel({}), localModel({ tag: "llava:7b", label: "LLaVA 7B", tools: false, vision: true })],
  hosted: [],
  ...over,
});

const openRouter = (over: Partial<SystemLLMStatus> = {}): SystemLLMStatus => ({
  local: false,
  reachable: true,
  provider: "openai",
  model: "",
  catalog: [],
  hosted: [
    {
      key: "openrouter",
      label: "OpenRouter",
      baseUrl: "https://openrouter.ai/api/v1",
      keysUrl: "https://openrouter.ai/keys",
      active: true,
      keyConfigured: true,
      modelsLive: true,
      models: [
        { id: "qwen/qwen3-32b", label: "Qwen3 32B", tools: true, vision: false },
        { id: "openai/gpt-4o", label: "GPT-4o", tools: true, vision: true },
      ],
    },
  ],
  ...over,
});

const mount = (props: ComponentProps<typeof AiModelSettings>) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterHarness initialPath="/settings" content={<AiModelSettings {...props} />} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
};

const openVision = async () =>
  userEvent.click(await screen.findByRole("button", { name: /Advanced vision model/ }));

describe("AiModelSettings", () => {
  it("says the status couldn't load and asks again on retry", async () => {
    let asked = 0;
    server.use(
      http.get("*/v1/system/llm", () => {
        asked += 1;
        return HttpResponse.json({ title: "AI status unavailable" }, { status: 503 });
      }),
    );
    mount({ provider: "ollama" });

    expect(await screen.findByText("AI status unavailable")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(asked).toBe(2));
  });

  // The downloadable list belongs to a reachable Ollama, so nothing asks for it otherwise.
  it("asks an unreachable Ollama host to be fixed and fetches no download list", async () => {
    let discovered = 0;
    server.use(
      getSystemLlmStatusMockHandler(ollama({ reachable: false })),
      getSystemLlmDiscoverMockHandler(() => {
        discovered += 1;
        return { models: [], sourceOk: true };
      }),
    );
    mount({ provider: "ollama" });

    expect(await screen.findByText(/Couldn't reach the Ollama host/)).toBeInTheDocument();
    expect(discovered).toBe(0);
  });

  it("confirms hosted credentials and asks for a lineup model when none is chosen", async () => {
    server.use(getSystemLlmStatusMockHandler(openRouter()), getSystemLlmTestMockHandler({ ok: true }));
    mount({ provider: "openai" });

    await userEvent.click(await screen.findByRole("button", { name: "Test provider credentials" }));

    expect(await screen.findByText(/OpenRouter credentials authorized/)).toBeInTheDocument();
    expect(screen.getByText(/Choose a tool-capable lineup model to finish AI setup/)).toBeInTheDocument();
  });

  it("shows only the lineup when the host stages no role settings", async () => {
    server.use(
      getSystemLlmStatusMockHandler(ollama()),
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
    );
    mount({ provider: "ollama" });

    expect(await screen.findByText("Qwen3 8B")).toBeInTheDocument();
    expect(screen.queryByText("Advanced vision model")).not.toBeInTheDocument();
    expect(screen.queryByText("Choose a lineup model")).not.toBeInTheDocument();
  });

  it("links to the TMDB connection while TMDB is missing", async () => {
    server.use(
      getSystemLlmStatusMockHandler(ollama()),
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
    );
    mount({ provider: "ollama", onRoleSettingChange: vi.fn(), tmdbConfigured: false });

    expect(await screen.findByText("TMDB grounding is still needed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Connect TMDB" })).toHaveAttribute(
      "href",
      "/settings/connections?focus=tmdb",
    );
  });

  describe("vision role", () => {
    it("stages a local vision model as Ollama", async () => {
      const staged = vi.fn();
      server.use(
        getSystemLlmStatusMockHandler(ollama()),
        getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      );
      mount({ provider: "ollama", onRoleSettingChange: staged });

      await openVision();
      await userEvent.click(screen.getByRole("button", { name: /LLaVA 7B/ }));

      expect(staged.mock.calls).toEqual([
        [SettingKey.FillerVisionProvider, "ollama"],
        [SettingKey.FillerVisionModel, "llava:7b"],
      ]);
    });

    // A hosted vision model lives on the lineup's own provider, which the settings store as
    // `inherit` with that model's id.
    it("stages a hosted vision model on the lineup's provider", async () => {
      const staged = vi.fn();
      server.use(getSystemLlmStatusMockHandler(openRouter()));
      mount({ provider: "openai", onRoleSettingChange: staged });

      await openVision();
      await userEvent.click(screen.getByRole("button", { name: /GPT-4o/ }));

      expect(staged.mock.calls).toEqual([
        [SettingKey.FillerVisionProvider, "inherit"],
        [SettingKey.FillerVisionModel, "openai/gpt-4o"],
      ]);
    });

    it("stages reuse as inherit with no model", async () => {
      const staged = vi.fn();
      server.use(
        getSystemLlmStatusMockHandler(ollama()),
        getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      );
      mount({
        provider: "ollama",
        visionProvider: "ollama",
        visionModel: "llava:7b",
        onRoleSettingChange: staged,
      });

      await openVision();
      await userEvent.click(screen.getByRole("button", { name: /Reuse the text-role choice/ }));

      expect(staged.mock.calls).toEqual([
        [SettingKey.FillerVisionProvider, "inherit"],
        [SettingKey.FillerVisionModel, ""],
      ]);
    });

    // Under a local lineup the stored inherit pair names an Ollama model, so that row is the
    // current choice.
    it("marks the stored model as chosen under a local lineup", async () => {
      server.use(
        getSystemLlmStatusMockHandler(ollama()),
        getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      );
      mount({
        provider: "ollama",
        visionProvider: "inherit",
        visionModel: "llava:7b",
        onRoleSettingChange: vi.fn(),
      });

      await openVision();

      expect(screen.getByRole("button", { name: /LLaVA 7B/ })).toBeDisabled();
      expect(screen.getByRole("button", { name: /Reuse the text-role choice/ })).toBeEnabled();
    });
  });
});
