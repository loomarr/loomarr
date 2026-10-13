import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
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
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, HttpResponse, http } from "msw";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { eraDates } from "@/lib/era-dates";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// Route-level coverage for #1895: policy edits on the Programming tab made inside one save round
// trip. The fake below keeps the backend's contract (internal/api/channels.go updateChannel): a
// PATCH whose `revision` is not the stored one is a 409 and changes nothing, an accepted PATCH
// replaces the policy wholesale and bumps the revision, and the response is the saved channel.
// Each accepted PATCH takes a moment, so a second blur lands while the first is still in flight.
type Stored = { revision: number; policy: ChannelPolicy };

const renderProgramming = (opts: { failWith?: number; onRead?: (stored: Stored) => void } = {}) => {
  const stored: Stored = { revision: 1, policy: {} };
  const patches: { revision: number; policy?: ChannelPolicy }[] = [];
  const conflicts: number[] = [];
  const current = () => channel({ id: "ch-1", revision: stored.revision, policy: stored.policy });
  server.use(
    getMeMockHandler(me()),
    // ⚠ Ahead of the by-id read, which would also answer `/v1/channels/now-next`.
    getChannelsNowNextMockHandler({ channels: [] }),
    // The Programming tab's other reads, empty: this suite is about the policy save path.
    getListDiscoveryFeedbackMockHandler([]),
    getListLibraryCollectionsMockHandler({ collections: [] }),
    getGetProgrammingVocabularyMockHandler({ what: [], when: [], how: [] }),
    getPreviewChannelCycleMockHandler(),
    getPreviewChannelProgrammingMockHandler(),
    http.get("*/v1/channels/:id", ({ params }) => {
      if (params.id !== "ch-1") return undefined;
      const body = current();
      opts.onRead?.(stored);
      return HttpResponse.json(body);
    }),
    http.patch("*/v1/channels/:id", async ({ request }) => {
      const body = (await request.json()) as { revision: number; policy?: ChannelPolicy };
      patches.push(body);
      await delay(80);
      if (opts.failWith) {
        return HttpResponse.json({ title: "Couldn't save the channel" }, { status: opts.failWith });
      }
      if (body.revision !== stored.revision) {
        conflicts.push(body.revision);
        return HttpResponse.json(
          { title: "Channel changed", detail: "This channel changed after you opened it." },
          { status: 409 },
        );
      }
      if (body.policy) stored.policy = body.policy;
      stored.revision += 1;
      return HttpResponse.json(current());
    }),
    ...appHandlers(),
  );

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
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
  return { stored, patches, conflicts };
};

// Types From, moves to To (the blur saves From), types To and leaves it (the blur saves To),
// all well inside the first PATCH's round trip.
const typeEraQuickly = async (from: string, to: string) => {
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText("From year"), from);
  await user.click(screen.getByLabelText("To year"));
  await user.keyboard(to);
  await user.tab();
};

describe("channel policy saves (#1895)", () => {
  beforeEach(() => vi.clearAllMocks());

  it("keeps both of two rapid Era edits", async () => {
    const { stored, conflicts } = renderProgramming();

    await typeEraQuickly("1990", "1999");

    await waitFor(() => expect(stored.policy.scope?.dates).toEqual(eraDates({ from: 1990, to: 1999 })));
    expect(conflicts).toEqual([]);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("re-applies the pending edit onto a channel another writer changed (409)", async () => {
    // Another writer (a reconcile, a second admin) saves the channel between the page's read and
    // the first PATCH: the page's revision is stale, and the other writer's field must survive.
    let bumped = false;
    const { stored } = renderProgramming({
      onRead: (s) => {
        if (bumped) return;
        bumped = true;
        queueMicrotask(() => {
          s.revision += 1;
          s.policy = { ...s.policy, ordering: "chronological" };
        });
      },
    });

    await typeEraQuickly("1990", "1999");

    await waitFor(() => expect(stored.policy.scope?.dates).toEqual(eraDates({ from: 1990, to: 1999 })));
    expect(stored.policy.ordering).toBe("chronological");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("shows an error when a save genuinely fails", async () => {
    const { patches } = renderProgramming({ failWith: 500 });

    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("From year"), "1990");
    await user.tab();

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't save the channel"));
    expect(patches).toHaveLength(1);
  });
});
