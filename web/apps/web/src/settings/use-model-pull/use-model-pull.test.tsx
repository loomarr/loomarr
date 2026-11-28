import * as systemApi from "@loomarr/api/endpoints/system";
import type { SystemLLMPullOperationDTO } from "@loomarr/api/models/systemLLMPullOperationDTO";
import { getSystemLlmPullMockHandler, getSystemLlmPullOperationMockHandler } from "@loomarr/api/msw";
import type { LlmPullEvent } from "@loomarr/core/events";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { delay, HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
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

// The operation the backend reports when it is read, and how many times it was read. The backend
// stores a download's outcome before publishing its frame, so a test moves it on with `advance`
// before sending that frame.
const serve = (initial: SystemLLMPullOperationDTO) => {
  let current = initial;
  const backend = {
    reads: 0,
    advance: (next: Partial<SystemLLMPullOperationDTO>) => {
      current = { ...current, ...next };
    },
  };
  server.use(
    getSystemLlmPullMockHandler({ jobId: initial.jobId }),
    getSystemLlmPullOperationMockHandler(() => {
      backend.reads++;
      return current;
    }),
  );
  return backend;
};

// One llm_pull frame for the download, as the backend sends it.
const pullFrame = (over: Partial<LlmPullEvent>): LlmPullEvent => ({
  jobId: "pull-1",
  model: "qwen3:8b",
  status: "pulling layer",
  percent: 0,
  completed: 0,
  total: 1000,
  ...over,
});

const renderPull = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
    </QueryClientProvider>
  );
  return { ...renderHook(() => useModelPull(), { wrapper }), client };
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

  it("follows the download's frames, then refreshes what is installed once it succeeds", async () => {
    const backend = serve(operation({ percent: 10 }));
    const { result, client } = renderPull();
    client.setQueryData(systemApi.getSystemLlmStatusQueryKey(), "installed models");

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling?.percent).toBe(10));

    act(() => FakeEventSource.broadcast("llm_pull", pullFrame({ percent: 55, completed: 550 })));
    await waitFor(() => expect(result.current.pulling).toEqual({ tag: "qwen3:8b", percent: 55 }));
    expect(client.getQueryState(systemApi.getSystemLlmStatusQueryKey())?.isInvalidated).toBe(false);

    backend.advance({ status: "success", percent: 100 });
    act(() => FakeEventSource.broadcast("llm_pull", pullFrame({ status: "success", percent: 100 })));
    await waitFor(() => expect(result.current.pulling).toBeUndefined());
    expect(result.current.error).toBeUndefined();
    expect(client.getQueryState(systemApi.getSystemLlmStatusQueryKey())?.isInvalidated).toBe(true);
  });

  it("reads the operation once and leaves the rest to its frames", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const backend = serve(operation({ percent: 10 }));
      const { result } = renderPull();

      act(() => result.current.start("qwen3:8b"));
      await waitFor(() => expect(result.current.pulling?.percent).toBe(10));
      await act(() => vi.advanceTimersByTimeAsync(5 * 60_000));

      expect(backend.reads).toBe(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("leaves the download alone when another download's frame arrives", async () => {
    serve(operation({ percent: 10 }));
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling?.percent).toBe(10));
    act(() => FakeEventSource.broadcast("llm_pull", pullFrame({ jobId: "pull-2", status: "error" })));

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(result.current.pulling).toEqual({ tag: "qwen3:8b", percent: 10 });
    expect(result.current.error).toBeUndefined();
  });

  it("says why a download failed when its frame reports the error", async () => {
    const backend = serve(operation({ percent: 30 }));
    const { result } = renderPull();

    act(() => result.current.start("qwen3:8b"));
    await waitFor(() => expect(result.current.pulling?.percent).toBe(30));
    backend.advance({ status: "error", percent: -1, error: "disk full" });
    act(() =>
      FakeEventSource.broadcast("llm_pull", pullFrame({ status: "error", percent: -1, error: "disk full" })),
    );

    await waitFor(() => expect(result.current.error).toEqual(new Error("disk full")));
    expect(result.current.pulling).toBeUndefined();
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
