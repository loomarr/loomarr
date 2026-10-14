import { getSettingsListMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { MAX_BACKOFF_MS, POLL_MS, pollDelay, useAiReconnect } from "./use-ai-reconnect";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// The one signal under test: GET /v1/settings' computed `features.suggestions`. Each read is
// counted so a test can prove a poll did — or did not — happen.
const stubFeatures = (initial: boolean) => {
  const state = { suggestions: initial, reads: 0 };
  server.use(
    getSettingsListMockHandler(() => {
      state.reads += 1;
      return { settings: [], features: { suggestions: state.suggestions } };
    }),
  );
  return state;
};

const setHidden = (hidden: boolean) => {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
  document.dispatchEvent(new Event("visibilitychange"));
};

const tick = async (ms: number) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

describe("useAiReconnect", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => {
    setHidden(false);
    vi.useRealTimers();
  });

  it("turns true when AI goes from unavailable to available, then stops polling", async () => {
    const features = stubFeatures(false);
    const { result } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });

    await waitFor(() => expect(features.reads).toBe(1));
    expect(result.current).toBe(false);

    await tick(POLL_MS);
    await waitFor(() => expect(features.reads).toBe(2));
    expect(result.current).toBe(false);

    features.suggestions = true;
    await tick(POLL_MS);
    await waitFor(() => expect(result.current).toBe(true));

    const reads = features.reads;
    await tick(POLL_MS * 4);
    expect(features.reads).toBe(reads);
  });

  it("never announces a reconnect when AI was already on as the builder opened", async () => {
    const features = stubFeatures(true);
    const { result } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });

    await waitFor(() => expect(features.reads).toBe(1));
    await tick(POLL_MS * 4);

    expect(result.current).toBe(false);
    expect(features.reads).toBe(1);
  });

  it("does not poll when disabled (a member, or 'Keep building manually')", async () => {
    const features = stubFeatures(false);
    renderHook(() => useAiReconnect(false), { wrapper: makeWrapper() });

    await tick(POLL_MS * 4);
    expect(features.reads).toBe(0);
  });

  it("stops polling once the builder unmounts", async () => {
    const features = stubFeatures(false);
    const { unmount } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));

    unmount();
    await tick(POLL_MS * 4);
    expect(features.reads).toBe(1);
  });

  it("pauses while the page is hidden and catches up when it is shown again", async () => {
    const features = stubFeatures(false);
    const { result } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));

    setHidden(true);
    features.suggestions = true;
    await tick(POLL_MS * 4);
    expect(features.reads).toBe(1);
    expect(result.current).toBe(false);

    setHidden(false);
    await tick(POLL_MS);
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("backs off on consecutive failures up to a minute, and resets on success", () => {
    expect(pollDelay(0)).toBe(POLL_MS);
    expect(pollDelay(1)).toBe(POLL_MS * 2);
    expect(pollDelay(2)).toBe(MAX_BACKOFF_MS);
    expect(pollDelay(9)).toBe(MAX_BACKOFF_MS);
  });

  it("waits longer after each failed read", async () => {
    let reads = 0;
    server.use(
      http.get("*/v1/settings", () => {
        reads += 1;
        return HttpResponse.json({ title: "Settings unavailable" }, { status: 500 });
      }),
    );
    renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(reads).toBe(1));

    // One failure: the next read is 30 s out, not 15.
    await tick(POLL_MS + 100);
    expect(reads).toBe(1);
    await tick(POLL_MS);
    await waitFor(() => expect(reads).toBe(2));

    // Two in a row: a minute, and no further.
    await tick(MAX_BACKOFF_MS - 1_000);
    expect(reads).toBe(2);
    await tick(1_100);
    await waitFor(() => expect(reads).toBe(3));
  });
});
