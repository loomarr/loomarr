import * as systemApi from "@loomarr/api/endpoints/system";
import type { SystemLLMPullOperationDTO } from "@loomarr/api/models/systemLLMPullOperationDTO";
import { getSystemLlmPullMockHandler, getSystemLlmPullOperationMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { delay, HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useModelPull } from "./use-model-pull";

const operation = (over: Partial<SystemLLMPullOperationDTO>): SystemLLMPullOperationDTO => ({
  jobId: "pull-1",
  model: "qwen3:8b",
  status: "running",
  percent: -1,
  completed: 0,
  total: 0,
  startedAt: "2026-09-01T12:00:00Z",
  updatedAt: "2026-09-01T12:00:01Z",
  ...over,
});

// The operation the backend reports, changed by each test as the download moves on.
const serve = (initial: SystemLLMPullOperationDTO) => {
  let current = initial;
  server.use(
    getSystemLlmPullMockHandler({ jobId: initial.jobId }),
    getSystemLlmPullOperationMockHandler(() => current),
  );
  return { advance: (next: SystemLLMPullOperationDTO) => (current = next) };
};

const renderPull = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { ...renderHook(() => useModelPull(), { wrapper }), invalidate };
};

describe("useModelPull", () => {
  it("shows the requested model before the backend answers, then the operation's progress", async () => {
    server.use(
      getSystemLlmPullMockHandler(async () => {
        await delay(50);
        return { jobId: "pull-1" };
      }),
      getSystemLlmPullOperationMockHandler(operation({ percent: 42 })),
    );
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling).toEqual({ tag: "qwen3:8b" }));

    await waitFor(() => expect(result.current.pulling).toEqual({ tag: "qwen3:8b", percent: 42 }));
    expect(result.current.error).toBeUndefined();
  });

  // percent -1 is the backend's "size not known yet" sentinel, not a percentage.
  it("shows no percent while the size is unknown", async () => {
    serve(operation({ percent: -1 }));
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));

    await waitFor(() => expect(result.current.pulling).toEqual({ tag: "qwen3:8b", percent: undefined }));
  });

  it("clears progress and refreshes what is installed once the download succeeds", async () => {
    const backend = serve(operation({ percent: 90 }));
    const { result, invalidate } = renderPull();

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling?.percent).toBe(90));
    backend.advance(operation({ status: "success", percent: 100 }));

    await waitFor(() => expect(result.current.pulling).toBeUndefined(), { timeout: 3_000 });
    expect(result.current.error).toBeUndefined();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: systemApi.getSystemLlmStatusQueryKey() });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: systemApi.getSystemLlmDiscoverQueryKey() });
  });

  it("says why a download failed and stops showing progress", async () => {
    serve(operation({ status: "error", error: "no space left on device" }));
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));

    await waitFor(() => expect(result.current.error).toEqual(new Error("no space left on device")));
    expect(result.current.pulling).toBeUndefined();
  });

  it("names the model when a failed operation gives no reason", async () => {
    serve(operation({ status: "error" }));
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));

    await waitFor(() => expect(result.current.error).toEqual(new Error("Downloading qwen3:8b failed.")));
  });

  it("reports a refused request and clears the error on the next try", async () => {
    server.use(
      http.post("*/v1/system/llm/pull", () =>
        HttpResponse.json({ title: "Ollama is busy" }, { status: 409 }),
      ),
    );
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.error).toBeDefined());
    expect(result.current.pulling).toBeUndefined();

    serve(operation({ percent: 5 }));
    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling?.percent).toBe(5));
    expect(result.current.error).toBeUndefined();
  });
});
