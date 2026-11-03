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

// The Programming tab reads the guide only for its timezone. Its window comes from
// `useGuideWindow` like Home, Watch and the Guide: pinned while the clock runs, slid near its end,
// and kept on screen (previous data) while the slid window loads.

afterEach(() => vi.useRealTimers());

describe("Programming tab guide window", () => {
  it("asks for the guide once while the clock runs, then again only after the window slides", async () => {
    const guideRequests: string[] = [];
    server.use(
      http.get("*/v1/guide", ({ request }) => {
        guideRequests.push(new URL(request.url).search);
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
    await waitFor(() => expect(guideRequests).toHaveLength(1));

    await act(async () => vi.advanceTimersByTime(2 * 60 * 60_000));
    expect(guideRequests).toHaveLength(1);

    await act(async () => vi.advanceTimersByTime(90 * 60_000));
    await waitFor(() => expect(guideRequests).toHaveLength(2));
    expect(guideRequests[1]).not.toBe(guideRequests[0]);
  });
});
