import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";
import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";
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

// Route-level coverage for the channel page's one write path (#1895, #1899): edits from different
// editors (policy fields, the lineup editor, the header's name) made inside one save round trip.
// The fake keeps the backend's contract (internal/api/channels.go updateChannel): a PATCH whose
// `revision` is not the stored one is a 409 "Channel changed" and changes nothing, an accepted
// PATCH sets only the fields it carries (the policy and lineup wholesale) and bumps the revision,
// and the response is the saved channel. Each accepted PATCH takes a moment, so later edits land
// while the first is still in flight.
type Stored = {
  revision: number;
  name: string;
  number: number;
  policy: ChannelPolicy;
  lineup: LineupEntryDTO[];
};

const velvet: LineupEntryDTO = { key: "movie:tmdb:90000949", name: "The Velvet Cartographer", year: 1995 };
const choir: LineupEntryDTO = { key: "movie:tmdb:90009426", name: "The Seventh Satellite Choir", year: 1991 };

const stale = () =>
  HttpResponse.json(
    { title: "Channel changed", detail: "This channel changed after you opened it." },
    { status: 409 },
  );

const renderChannel = (
  opts: {
    fail?: (body: UpdateChannelInputBody) => Response | undefined;
    holdFirst?: boolean;
    onRead?: (stored: Stored) => void;
  } = {},
) => {
  const stored: Stored = {
    revision: 1,
    name: "Late Night Noir",
    number: 1,
    policy: {},
    lineup: [velvet, choir],
  };
  const patches: UpdateChannelInputBody[] = [];
  const conflicts: number[] = [];
  // `holdFirst` keeps the first PATCH open until the test releases it, so the later edits are
  // certain to land inside its round trip however fast or slow the typing runs.
  let release = () => {};
  const firstHeld = opts.holdFirst ? new Promise<void>((resolve) => (release = resolve)) : Promise.resolve();
  const current = () => channel({ id: "ch-1", ...stored });
  server.use(
    getMeMockHandler(me()),
    // ⚠ Ahead of the by-id read, which would also answer `/v1/channels/now-next`.
    getChannelsNowNextMockHandler({ channels: [] }),
    // The Programming tab's other reads, empty: this suite is about the save path.
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
      const body = (await request.json()) as UpdateChannelInputBody;
      patches.push(body);
      if (patches.length === 1) await firstHeld;
      await delay(80);
      const failure = opts.fail?.(body);
      if (failure) return failure;
      if (body.revision !== stored.revision) {
        conflicts.push(body.revision);
        return stale();
      }
      if (body.name !== undefined) stored.name = body.name;
      if (body.number !== undefined) stored.number = body.number;
      if (body.policy) stored.policy = body.policy;
      if (body.lineup) stored.lineup = body.lineup;
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
  return { stored, patches, conflicts, release };
};

// Types From, moves to To (the blur saves From), types To and leaves it (the blur saves To).
const typeEra = async (user: ReturnType<typeof userEvent.setup>, from: string, to: string) => {
  await user.type(await screen.findByLabelText("From year"), from);
  await user.click(screen.getByLabelText("To year"));
  await user.keyboard(to);
  await user.tab();
};

const rename = async (user: ReturnType<typeof userEvent.setup>, name: string) => {
  const field = screen.getByLabelText("Channel name");
  await user.clear(field);
  await user.type(field, `${name}{Enter}`);
};

describe("channel saves", () => {
  beforeEach(() => vi.clearAllMocks());

  it("keeps every edit made across editors inside one round trip (#1899)", async () => {
    const { stored, patches, conflicts, release } = renderChannel({ holdFirst: true });
    const user = userEvent.setup();

    // A policy edit, a lineup remove and a rename, all while the first save is still open.
    await typeEra(user, "1990", "1999");
    await user.click(screen.getByRole("button", { name: "Remove The Seventh Satellite Choir" }));
    await rename(user, "Midnight Noir");
    release();

    await waitFor(() =>
      expect(stored).toMatchObject({
        name: "Midnight Noir",
        lineup: [velvet],
        policy: { scope: { dates: eraDates({ from: 1990, to: 1999 }) } },
      }),
    );
    // Never two PATCHes at once: each carried the revision the previous response returned.
    expect(patches.map((p) => p.revision)).toEqual(patches.map((_, i) => i + 1));
    expect(conflicts).toEqual([]);
    expect(toast.error).not.toHaveBeenCalled();
    // The header shows the saved name once its save lands.
    await waitFor(() => expect(screen.getByLabelText("Channel name")).toHaveValue("Midnight Noir"));
  });

  it("keeps both of two rapid Era edits (#1895)", async () => {
    const { stored, patches, conflicts, release } = renderChannel({ holdFirst: true });

    await typeEra(userEvent.setup(), "1990", "1999");
    // Both blurs happened while the first save was still open.
    release();

    await waitFor(() => expect(stored.policy.scope?.dates).toEqual(eraDates({ from: 1990, to: 1999 })));
    // The second PATCH waited for the first and carried the revision it returned.
    expect(patches.map((p) => p.revision)).toEqual([1, 2]);
    expect(conflicts).toEqual([]);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("re-applies pending edits onto a channel another writer changed (409)", async () => {
    // Another writer (a reconcile, a second admin) saves the channel between the page's read and
    // the first PATCH: the page's revision is stale, and the other writer's field must survive.
    let bumped = false;
    const { stored } = renderChannel({
      onRead: (s) => {
        if (bumped) return;
        bumped = true;
        queueMicrotask(() => {
          s.revision += 1;
          s.policy = { ...s.policy, ordering: "chronological" };
        });
      },
    });
    const user = userEvent.setup();

    await typeEra(user, "1990", "1999");
    await user.click(screen.getByRole("button", { name: "Remove The Seventh Satellite Choir" }));

    await waitFor(() => expect(stored.policy.scope?.dates).toEqual(eraDates({ from: 1990, to: 1999 })));
    await waitFor(() => expect(stored.lineup).toEqual([velvet]));
    expect(stored.policy.ordering).toBe("chronological");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("shows an error when a save genuinely fails, and the editor falls back to what is saved", async () => {
    const { stored, patches } = renderChannel({
      fail: () => HttpResponse.json({ title: "Couldn't save the channel" }, { status: 500 }),
    });
    const user = userEvent.setup();

    await screen.findByLabelText("From year");
    await user.click(screen.getByRole("button", { name: "Remove The Seventh Satellite Choir" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't save the channel"));
    expect(patches).toHaveLength(1);
    expect(stored.lineup).toEqual([velvet, choir]);
    expect(await screen.findByRole("button", { name: "Remove The Seventh Satellite Choir" })).toBeInTheDocument();
  });

  it("surfaces a number collision on the field instead of retrying it as a stale edit", async () => {
    // A taken number is a 409 too, but not "Channel changed": refetching cannot fix it.
    const { patches } = renderChannel({
      fail: (body) =>
        body.number === 2
          ? HttpResponse.json(
              { title: "Channel already exists", detail: "That channel number was claimed." },
              { status: 409 },
            )
          : undefined,
    });
    const user = userEvent.setup();

    const field = await screen.findByLabelText("Channel number");
    await user.clear(field);
    await user.type(field, "2{Enter}");

    expect(await screen.findByText("Channel already exists")).toBeInTheDocument();
    expect(patches).toHaveLength(1);
    expect(toast.error).toHaveBeenCalledWith("Channel already exists");
  });
});
