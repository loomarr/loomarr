import {
  getChannelsNowNextMockHandler,
  getGetProgrammingVocabularyMockHandler,
  getListDiscoveryFeedbackMockHandler,
  getListLibraryCollectionsMockHandler,
  getMeMockHandler,
  getPreviewChannelCycleMockHandler,
  getPreviewChannelProgrammingMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { act, render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// The Programming tab tells times in the household's timezone. It reads that one fact from the
// household read, once, rather than loading the Guide and re-reading it as the Guide's window slides.

afterEach(() => vi.useRealTimers());

describe("Programming tab household time", () => {
  it("reads the household timezone once and never loads the Guide for it", async () => {
    let householdReads = 0;
    let guideReads = 0;
    server.use(
      http.get("*/v1/household", () => {
        householdReads++;
        return HttpResponse.json({ timeZone: "UTC" });
      }),
      http.get("*/v1/guide", () => {
        guideReads++;
        return HttpResponse.json({ channels: [], fromMs: 0, toMs: 0, timezone: "UTC" } as never);
      }),
      getMeMockHandler(me()),
      getChannelsNowNextMockHandler({ channels: [] }),
      getListDiscoveryFeedbackMockHandler([]),
      getListLibraryCollectionsMockHandler({ collections: [] }),
      getGetProgrammingVocabularyMockHandler({ what: [], when: [], how: [] }),
      getPreviewChannelCycleMockHandler(),
      getPreviewChannelProgrammingMockHandler(),
      http.get("*/v1/channels/:id", ({ params }) =>
        params.id === "ch-1" ? HttpResponse.json(channel({ id: "ch-1" })) : undefined,
      ),
      ...appHandlers(),
    );
    // Only the clock is faked: msw and Testing Library keep their real timers.
    vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const router = createRouter({
      routeTree,
      context: { queryClient },
      history: createMemoryHistory({ initialEntries: ["/channels/ch-1/programming"] }),
    });
    render(
      <LoomarrProvider theme="dark">
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </LoomarrProvider>,
    );
    await screen.findByRole("heading", { name: "Programming" });
    await waitFor(() => expect(householdReads).toBe(1));

    // Hours pass, past where the Guide's window would have slid.
    await act(async () => vi.advanceTimersByTime(4 * 60 * 60_000));
    expect(householdReads).toBe(1);
    expect(guideReads).toBe(0);
  });
});
