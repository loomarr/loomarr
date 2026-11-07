import {
  getSecretRegenerateMockHandler,
  getSystemLlmDiscoverMockHandler,
  getSystemLlmPullMockHandler,
  getSystemLlmPullOperationMockHandler,
  getSystemLlmSelectMockHandler,
  getSystemLlmStatusMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { AiModelSettings } from "@/settings/ai-model-settings";
import { SecretsSettings } from "@/settings/secrets-settings";
import { useSettingsEdits } from "@/settings/settings-edits";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";

// config-design §5 names FOUR inline-commit exceptions, and they are all verbs: select a model,
// pull a model, regenerate a secret, run a job now. Each commits immediately and deliberately
// sits OUTSIDE the cross-tab save bar, because you do not stage an action — a Save button next
// to "Run now" is nonsense, and staging "regenerate a secret" would be worse than nonsense
// since the operation is destructive the moment it runs.
//
// V9 lifted the edit buffer into the settings layout, which made it newly easy to sweep these in
// "for consistency". The buffer's hook throws outside its provider, so each surface here is
// mounted with NO SettingsEditsProvider: one that started reading the buffer fails to render, and
// each must still put its request on the wire the moment it is clicked. "Run a job now" is held
// the same way by TasksPage's own suite (settings/tasks-page), which mounts it without the
// provider and asserts the run request.
const mount = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>{ui}</TooltipProvider>
    </QueryClientProvider>,
  );
};

describe("the inline-commit exceptions stay out of the save bar", () => {
  it("the save bar's buffer is unavailable outside the settings layout", () => {
    const ReadsBuffer = () => {
      useSettingsEdits();
      return null;
    };
    expect(() => render(<ReadsBuffer />)).toThrow(/SettingsEditsProvider/);
  });

  it("regenerating a secret commits immediately", async () => {
    const regenerated: string[] = [];
    server.use(
      getSecretRegenerateMockHandler(({ params }) => {
        regenerated.push(String(params.name));
        return { value: "new-token-value" };
      }),
    );
    mount(<SecretsSettings />);

    const [apiToken] = screen.getAllByRole("button", { name: /^Regenerate$/ });
    await userEvent.click(apiToken as HTMLElement);
    await userEvent.click(screen.getByRole("button", { name: /Regenerate anyway/ }));

    await waitFor(() => expect(regenerated).toEqual(["api_token"]));
    expect(await screen.findByText("new-token-value")).toBeInTheDocument();
  });

  it("selecting and pulling a model each commit immediately", async () => {
    const selected: unknown[] = [];
    const pulled: unknown[] = [];
    const model = {
      approxVramGiB: 5,
      fit: "fits" as const,
      recommended: false,
      runtimeOk: true,
      tools: true,
      vision: false,
      why: "Good fit.",
    };
    server.use(
      getSystemLlmStatusMockHandler({
        local: true,
        reachable: true,
        provider: "ollama",
        model: "",
        catalog: [
          { ...model, tag: "qwen3:8b", label: "Qwen3 8B", pulled: true },
          { ...model, tag: "llama3.2:3b", label: "Llama 3.2 3B", pulled: false },
        ],
        hosted: [],
      }),
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      getSystemLlmSelectMockHandler(async ({ request }) => {
        selected.push(await request.json());
        return undefined as never;
      }),
      getSystemLlmPullMockHandler(async ({ request }) => {
        pulled.push(await request.json());
        return { jobId: "pull-1" };
      }),
      getSystemLlmPullOperationMockHandler({
        jobId: "pull-1",
        model: "llama3.2:3b",
        status: "running",
        percent: 10,
        completed: 0,
        total: 0,
        startedAt: "2026-09-01T12:00:00Z",
        updatedAt: "2026-09-01T12:00:01Z",
      }),
    );
    mount(<AiModelSettings provider="ollama" />);

    await userEvent.click(await screen.findByRole("button", { name: "Use this" }));
    await waitFor(() => expect(selected).toEqual([{ model: "qwen3:8b" }]));

    await userEvent.click(screen.getByRole("button", { name: /Download/ }));
    await waitFor(() => expect(pulled).toEqual([{ model: "llama3.2:3b" }]));
  });
});
