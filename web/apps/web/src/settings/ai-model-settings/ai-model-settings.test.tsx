import type { SystemLLMStatus } from "@loomarr/api";
import * as systemApi from "@loomarr/api/endpoints/system";
import {
  getSystemLlmDiscoverMockHandler,
  getSystemLlmStatusMockHandler,
  getSystemLlmVerifyMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { AiModelSettings } from "./ai-model-settings";

const status = (over: Partial<SystemLLMStatus> = {}): SystemLLMStatus => ({
  provider: "ollama",
  model: "qwen3:8b",
  local: true,
  reachable: true,
  configured: true,
  toolCapability: "verified",
  semanticallyCertified: false,
  catalog: [],
  hosted: [],
  ...over,
});

const renderSettings = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <AiModelSettings provider="ollama" />
    </QueryClientProvider>,
  );
  return { client, ...view };
};

const stubStatus = (value: SystemLLMStatus, verifyCalls: unknown[]) =>
  server.use(
    getSystemLlmStatusMockHandler(value),
    getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
    getSystemLlmVerifyMockHandler(async ({ request }) => {
      verifyCalls.push(await request.json());
      return {
        provider: value.provider,
        model: value.model,
        reachable: true,
        toolCapability: "verified",
        semanticallyCertified: false,
      };
    }),
  );

describe("AiModelSettings readiness", () => {
  it("keeps configuration, reachability, tool capability, and semantic certification distinct", async () => {
    const verifyCalls: unknown[] = [];
    stubStatus(status(), verifyCalls);
    const { client } = renderSettings();

    const readiness = await screen.findByRole("region", { name: "AI readiness" });
    expect(readiness).toHaveTextContent("Configured");
    expect(readiness).toHaveTextContent("Reachable");
    expect(readiness).toHaveTextContent("Tool calling");
    expect(readiness).toHaveTextContent("Verified");
    expect(readiness).toHaveTextContent("Semantic curation");
    expect(readiness).toHaveTextContent("Not certified");
    expect(readiness).toHaveTextContent(/tool verification does not certify curation quality/i);

    await client.refetchQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() });
    expect(verifyCalls).toHaveLength(0);
  });

  it("runs verification only after the operator presses the explicit action", async () => {
    const verifyCalls: unknown[] = [];
    stubStatus(
      status({
        toolCapability: "unverified",
        catalog: [
          {
            tag: "qwen3:8b",
            label: "Qwen 3 8B",
            approxVramGiB: 6,
            fit: "fits",
            pulled: true,
            runtimeOk: true,
            tools: false,
            toolCapability: "unverified",
            recommended: true,
            why: "Best fit for this machine.",
          },
        ],
      }),
      verifyCalls,
    );
    renderSettings();

    expect(await screen.findByText("Tools unverified")).toBeInTheDocument();
    expect(verifyCalls).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: /verify tool calling/i }));
    await waitFor(() => expect(verifyCalls).toEqual([{ provider: "ollama", model: "qwen3:8b" }]));
  });
});
