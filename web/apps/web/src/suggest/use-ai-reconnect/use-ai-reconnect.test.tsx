import { getSettingsListMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { server } from "@/test/msw/server";
import { useAiReconnect } from "./use-ai-reconnect";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
    </QueryClientProvider>
  );
};

// The one signal under test: GET /v1/settings' computed `features.suggestions`. Each read is
// counted so a test can prove a read did — or did not — happen.
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

// A committed settings write, as the backend announces it.
const settingsSaved = () => act(() => FakeEventSource.broadcast("settings", {}));

// Lets any read a frame started land, so a test can say none did.
const settle = () => act(() => new Promise((resolve) => setTimeout(resolve, 20)));

describe("useAiReconnect", () => {
  it("turns true when a settings write turns AI on, then lets the read go", async () => {
    const features = stubFeatures(false);
    const { result } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));
    expect(result.current).toBe(false);

    // A write that leaves AI off is read and changes nothing.
    settingsSaved();
    await waitFor(() => expect(features.reads).toBe(2));
    expect(result.current).toBe(false);

    features.suggestions = true;
    settingsSaved();
    await waitFor(() => expect(result.current).toBe(true));

    const reads = features.reads;
    settingsSaved();
    await settle();
    expect(features.reads).toBe(reads);
  });

  it("never announces a reconnect when AI was already on as the builder opened", async () => {
    const features = stubFeatures(true);
    const { result } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));

    settingsSaved();
    await settle();
    expect(result.current).toBe(false);
    expect(features.reads).toBe(1);
  });

  it("reads nothing when disabled (a member, or 'Keep building manually')", async () => {
    const features = stubFeatures(false);
    renderHook(() => useAiReconnect(false), { wrapper: makeWrapper() });

    settingsSaved();
    await settle();
    expect(features.reads).toBe(0);
  });

  it("reads nothing more once the builder unmounts", async () => {
    const features = stubFeatures(false);
    const { unmount } = renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));

    unmount();
    settingsSaved();
    await settle();
    expect(features.reads).toBe(1);
  });

  it("does not poll between writes", async () => {
    const features = stubFeatures(false);
    renderHook(() => useAiReconnect(true), { wrapper: makeWrapper() });
    await waitFor(() => expect(features.reads).toBe(1));

    await act(() => new Promise((resolve) => setTimeout(resolve, 1_200)));
    expect(features.reads).toBe(1);
  });
});
