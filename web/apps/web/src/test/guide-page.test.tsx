import type { MeBody, TitleDTO } from "@loomarr/api";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import {
  getChannelGuideMockHandler,
  getChannelUpcomingMockHandler,
  getDeleteChannelMockHandler,
  getEnqueueTitleMockHandler,
  getGetProposalJobMockHandler,
  getListChannelsMockHandler,
  getListTitlesMockHandler,
  getMeMockHandler,
  getMyChannelsMockHandler,
  getPreviewChannelPodsMockHandler,
  getSettingsListMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { setting } from "@/test/fixtures/settings";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// The Guide — the channels surface — through the REAL route tree.
//
// Was `channels-board.test.tsx` against `/channels`. That route folded into `/guide` (§12):
// one surface answers "what do I have" and "what is on", and it owns ORIGINATION. The
// assertions that survived the move are the ones about behavior rather than layout — the
// per-row actions menu, the inline "Add a channel" door, and the absence of any manual
// rebuild control (§9 self-maintaining). The card-list-shaped ones did not survive, because
// the cards did not.
const MEMBER: MeBody = me({ id: "u2", name: "Kid", role: "member", autoApprove: false });

// ⚠ Both of these were written inline as ten-field objects and were missing `lineup`, which
// ChannelDTO requires. `channel()` carries the full required set.
const CHANNELS = [
  channel({
    id: "ch-live",
    name: "Saturday Cartoons",
    number: 42,
    programCount: 10,
    breakCount: 4,
    slotCount: 14,
  }),
  channel({
    id: "ch-part",
    name: "Late Night",
    number: 43,
    programCount: 3,
    pendingCount: 7,
    slotCount: 10,
  }),
];

const TITLES: TitleDTO[] = [
  { key: "movie:tmdb:90000001", mediaType: "movie", state: "available", name: "Landed" },
  { key: "movie:tmdb:90000002", mediaType: "movie", state: "downloading", name: "Coming" },
  { key: "movie:tmdb:90000003", mediaType: "movie", state: "unavailable", name: "Gave Up", tmdbId: 90000003 },
];

// GET /v1/guide's shape, read off the generated GuideChannelTimeline/GuideAiring DTOs
// rather than remembered — the same rule the fixtures carry, and the one that caught two
// wrong proposal shapes in 13.4e.
const NOW = 1_700_000_000_000;
const GUIDE = {
  fromMs: NOW,
  toMs: NOW + 4 * 3_600_000,
  channels: [
    {
      channelId: "ch-live",
      name: "Saturday Cartoons",
      number: 42,
      status: "live" as const,
      pendingCount: 0,
      airings: [
        {
          kind: "program" as const,
          scheduleBlockId: "block_western",
          title: "Space Western",
          startMs: NOW,
          stopMs: NOW + 7_200_000,
          runtimeMs: 7_200_000,
        },
      ],
    },
  ],
};

// ⚠ GET /v1/titles is a single-state FILTER and it 400s without a `state` param — mirroring
// `internal/api/titles.go`, so a caller that forgets the param fails HERE exactly as it would in
// production. Answering every titles request with the full set is what let the Board ship a
// param-less call that 400s live while the suite stayed green.
//
// This is the one case in the file that cannot be generated: the spec declares errors via
// `default:` (RFC 7807) with no explicit 400, so orval has no status to emit a handler for. The
// shape used is MSW's PASS-THROUGH — a resolver that returns nothing declines the request and the
// next matching handler takes it — so the 400 is a hand-written GUARD in front of the generated
// success path, rather than a hand-written replacement for it.
const titlesRequireState = () =>
  http.get("*/v1/titles", ({ request }) => {
    if (new URL(request.url).searchParams.get("state")) return undefined;
    return HttpResponse.json(
      { title: "Bad Request", detail: "state query param is required" },
      { status: 400 },
    );
  });

// `empty` serves a guide with NO channels, which is the fresh-install state the "Dead air"
// empty state and the hidden header door both hang off.
const stubGuide = (who: MeBody = me(), opts: { empty?: boolean } = {}) => {
  const enqueued: unknown[] = [];

  server.use(
    getMeMockHandler(who),
    getChannelGuideMockHandler(opts.empty ? { ...GUIDE, channels: [] } : GUIDE),
    getListChannelsMockHandler({ channels: opts.empty ? [] : CHANNELS }),
    titlesRequireState(),
    getListTitlesMockHandler(({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      return { titles: TITLES.filter((t) => t.state === state) };
    }),
    getEnqueueTitleMockHandler(async ({ request }) => {
      enqueued.push(await request.json());
      return { key: "movie:tmdb:90000003", mediaType: "movie", state: "wanted" };
    }),
    // tunarr.url set → the list's Rebuild button is enabled (it's gated on Tunarr being
    // connected, so a rebuild can't 501). ⚠ The old entry supplied four of SettingEntry's eight
    // required fields; `setting()` carries the rest.
    getSettingsListMockHandler({ features: {}, settings: [setting({ key: "tunarr.url" })] }),
    // A channel's page, which the tests that open a channel's info land on, asks for these. Once
    // its route chunk is loaded by an earlier test it asks before the test ends.
    getChannelUpcomingMockHandler({ upcoming: [] }),
    getPreviewChannelPodsMockHandler({ entries: [], totalMs: 0, matchLevel: "exact" }),
    // Spread LAST — see handlers.ts: MSW takes the FIRST match and `use()` prepends.
    ...appHandlers(),
  );

  return { enqueued };
};

// The Guide prefetches neighbouring windows, so a test can end with requests still in flight.
// Such a request would reach the NEXT test's handlers and be recorded there (the slow-window
// stub would take it for its own "first" window and gate the real one), so every test drains
// its clients before it ends.
const clients: QueryClient[] = [];
afterEach(async () => {
  await Promise.all(clients.splice(0).map((c) => vi.waitFor(() => expect(c.isFetching()).toBe(0))));
});

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(queryClient);
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  // Returns the render RESULT so a test can scope its queries to its own tree. `screen`
  // searches the shared document.body, and a query that reaches a neighbouring test's markup
  // clicks a detached button, which silently does nothing.
  // LoomarrProvider as main.tsx mounts it: the grid and the programme card are design-system views.
  const view = render(
    <LoomarrProvider theme="dark">
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </LoomarrProvider>,
  );
  return Object.assign(view, { router });
};

// The guide opens on the Now/Next list (#2787); these tests are about the full grid.
const GRID = "/guide?view=grid";

// The list asks what is on at the real clock, so its guide is served around it.
const onNow = (channels: { channelId: string; name: string; number: number; title: string }[]) => {
  const now = Date.now();
  return getChannelGuideMockHandler({
    fromMs: now - 1_800_000,
    toMs: now + 3 * 3_600_000,
    channels: channels.map(({ title, ...ch }) => ({
      ...ch,
      status: "live" as const,
      pendingCount: 0,
      airings: [
        {
          kind: "program" as const,
          scheduleBlockId: `block_${ch.channelId}`,
          title,
          startMs: now - 600_000,
          stopMs: now + 3_000_000,
        },
      ],
    })),
  });
};

describe("Guide's Now & next list", () => {
  it("is what a desktop opens on, with the full grid one click away", async () => {
    const user = userEvent.setup();
    stubGuide();
    server.use(
      onNow([{ channelId: "ch-live", name: "Saturday Cartoons", number: 42, title: "Space Western" }]),
    );
    const view = renderAt("/guide");

    expect(await view.findByRole("heading", { name: "What's on" })).toBeInTheDocument();
    expect(await view.findByText("Space Western")).toBeInTheDocument();
    // No grid, so no span picker and no grid row menu.
    expect(view.queryByRole("button", { name: "Show 4 hours" })).not.toBeInTheDocument();
    expect(view.queryByRole("button", { name: /actions for saturday cartoons/i })).not.toBeInTheDocument();

    await user.click(view.getByRole("tab", { name: "Full grid" }));
    expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    expect(view.router.state.location.search).toEqual({ view: "grid" });
  });

  it("opens a channel's info from its row's ⋯", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt("/guide");

    await user.click(await view.findByRole("button", { name: "Channel info for Saturday Cartoons" }));
    await waitFor(() => expect(view.router.state.location.pathname).toBe("/channels/ch-live/info"));
  });

  it("puts the viewer's favourites first", async () => {
    stubGuide();
    server.use(
      onNow([
        { channelId: "ch-live", name: "Saturday Cartoons", number: 42, title: "Space Western" },
        { channelId: "ch-fav", name: "Movie Night", number: 99, title: "A Favourite Film" },
      ]),
      getMyChannelsMockHandler({
        favourites: [{ addedAt: "2026-11-01T00:00:00Z", channelId: "ch-fav" }],
        recent: [],
      }),
    );
    const view = renderAt("/guide");

    const favourite = await view.findByText("A Favourite Film");
    await waitFor(() =>
      expect(
        favourite.compareDocumentPosition(view.getByText("Space Western")) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy(),
    );
  });
});

describe("Guide", () => {
  it("is headed 'Guide' and shows what's on, from the guide endpoint", async () => {
    stubGuide();
    renderAt(GRID);
    // The page is headed for its nav item (#3413), and the grid view adds no second "Guide" under it.
    expect(await screen.findByRole("heading", { name: "Guide", level: 1 })).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { name: "Guide" })).toHaveLength(1);
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
  });

  // A failed refetch (the SSE bus invalidating every window) used to swap the whole page for an
  // error. The Guide on screen stays, and a Notice says it may be stale until a refresh lands.
  it("keeps the Guide on screen and says so when a refresh fails", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);
    expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();

    server.use(http.get("*/v1/guide", () => new HttpResponse(null, { status: 503 })));
    const isGuide = (query: { queryKey: readonly unknown[] }) =>
      String(query.queryKey[0]).includes("/v1/guide");
    await clients.at(-1)?.refetchQueries({ predicate: isGuide });

    expect(await view.findByText("Couldn't refresh the Guide")).toBeInTheDocument();
    expect(view.getByText("Showing what Loomarr had a moment ago.")).toBeInTheDocument();
    expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();

    server.resetHandlers();
    stubGuide();
    await user.click(view.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
    expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
  });

  // The notice's button is where the keyboard's focus sits when it is pressed. A retry that took
  // the notice away while it ran dropped focus to the page, and a retry that failed again brought
  // the notice back with nothing focused.
  it("keeps focus on the notice's Try again while it retries, and when the retry fails", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);
    expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();

    const isGuide = (query: { queryKey: readonly unknown[] }) =>
      String(query.queryKey[0]).includes("/v1/guide");
    server.use(http.get("*/v1/guide", () => new HttpResponse(null, { status: 503 })));
    await clients.at(-1)?.refetchQueries({ predicate: isGuide });
    const button = await view.findByRole("button", { name: "Try again" });
    button.focus();

    // The retry is held on its way to failing again.
    let answer = () => {};
    const held = new Promise<void>((resolve) => {
      answer = resolve;
    });
    server.use(
      http.get("*/v1/guide", async () => {
        await held;
        return new HttpResponse(null, { status: 503 });
      }),
    );
    await user.click(button);

    const retrying = await view.findByRole("button", { name: "Trying again…" });
    expect(retrying).toBe(button);
    expect(button).toHaveFocus();
    expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
    // A second press while it runs is the same retry.
    await user.click(button);

    answer();
    await waitFor(() => expect(button).toHaveAccessibleName("Try again"));
    expect(view.getAllByRole("button", { name: "Try again" })).toEqual([button]);
    expect(button).toHaveFocus();
  });

  // A retry that works takes the notice (or the error page) away, and the focus on its button goes
  // with it, to the page. Focus goes instead to the Guide's home: the grid's one Tab stop, or the
  // page's main region when no grid is drawn.
  describe("after a retry works", () => {
    const failGuide = () => server.use(http.get("*/v1/guide", () => new HttpResponse(null, { status: 503 })));
    const isGuide = (query: { queryKey: readonly unknown[] }) =>
      String(query.queryKey[0]).includes("/v1/guide");
    const gridStop = () => {
      const stop = document.activeElement;
      expect(stop, "focus is on the page, not a control").not.toBe(document.body);
      expect(stop).toHaveAttribute("tabindex", "0");
      return stop;
    };

    it("puts focus on the grid's Tab stop when a notice's retry works", async () => {
      const user = userEvent.setup();
      stubGuide();
      const view = renderAt(GRID);
      expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      failGuide();
      await clients.at(-1)?.refetchQueries({ predicate: isGuide });
      await user.click(await view.findByRole("button", { name: "Try again" }));
      await view.findByRole("button", { name: "Try again" });

      server.resetHandlers();
      stubGuide();
      await user.click(view.getByRole("button", { name: "Try again" }));

      await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
      await waitFor(() => expect(document.activeElement).not.toBe(document.body));
      expect(gridStop()).toHaveAccessibleName(/^Space Western, /);
    });

    it("puts focus on the grid's Tab stop when the error page's retry works", async () => {
      const user = userEvent.setup();
      stubGuide();
      failGuide();
      const view = renderAt(GRID);
      const button = await view.findByRole("button", { name: "Try again" });
      button.focus();

      // Held on its way to failing again: the button stays, and says so.
      let answer = () => {};
      const held = new Promise<void>((resolve) => {
        answer = resolve;
      });
      server.use(
        http.get("*/v1/guide", async () => {
          await held;
          return new HttpResponse(null, { status: 503 });
        }),
      );
      await user.click(button);
      expect(await view.findByRole("button", { name: "Trying again…" })).toBe(button);
      expect(button).toHaveFocus();
      answer();
      await waitFor(() => expect(button).toHaveAccessibleName("Try again"));
      expect(button).toHaveFocus();

      server.resetHandlers();
      stubGuide();
      await user.click(button);

      expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      await waitFor(() => expect(document.activeElement).not.toBe(document.body));
      expect(gridStop()).toHaveAccessibleName(/^Space Western, /);
    });

    it("puts focus on the page's main region when no grid is drawn (Now & next)", async () => {
      const user = userEvent.setup();
      stubGuide();
      server.use(http.get("*/v1/guide", () => new HttpResponse(null, { status: 503 })));
      const view = renderAt("/guide");
      const button = await view.findByRole("button", { name: "Try again" });
      button.focus();

      server.resetHandlers();
      stubGuide();
      server.use(
        onNow([{ channelId: "ch-live", name: "Saturday Cartoons", number: 42, title: "Space Western" }]),
      );
      await user.click(button);

      expect(await view.findByText("Space Western")).toBeInTheDocument();
      await waitFor(() => expect(document.activeElement).toBe(document.getElementById("main-content")));
    });

    it("leaves focus alone when the viewer has already moved it elsewhere", async () => {
      const user = userEvent.setup();
      stubGuide();
      const view = renderAt(GRID);
      expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      failGuide();
      await clients.at(-1)?.refetchQueries({ predicate: isGuide });
      const retry = await view.findByRole("button", { name: "Try again" });

      server.resetHandlers();
      stubGuide();
      let answer = () => {};
      const held = new Promise<void>((resolve) => {
        answer = resolve;
      });
      server.use(
        http.get("*/v1/guide", async () => {
          await held;
          return undefined;
        }),
      );
      await user.click(retry);
      const now = view.getByRole("button", { name: "NOW" });
      now.focus();
      answer();

      await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
      expect(now).toHaveFocus();
    });

    // The native Guide had a press armed that outlived a failed retry; the page keeps no such memory,
    // so a recovery nobody pressed for, after a retry that failed, still leaves the viewer's focus.
    it("leaves focus alone when a background refresh recovers after a retry that failed", async () => {
      const user = userEvent.setup();
      stubGuide();
      const view = renderAt(GRID);
      expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      failGuide();
      await clients.at(-1)?.refetchQueries({ predicate: isGuide });
      await user.click(await view.findByRole("button", { name: "Try again" }));
      await view.findByRole("button", { name: "Try again" });

      const now = view.getByRole("button", { name: "NOW" });
      now.focus();
      server.resetHandlers();
      stubGuide();
      await clients.at(-1)?.refetchQueries({ predicate: isGuide });

      await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
      expect(now).toHaveFocus();
    });
  });

  // #3411: the tab read a bare "Loomarr" here while every other page named itself.
  it("names its browser tab", async () => {
    stubGuide();
    renderAt(GRID);
    await screen.findByRole("heading", { name: "Guide", level: 1 });
    expect(document.title).toBe("Guide | Loomarr");
  });

  // #2727: a channel with no programme yet filled its row with unlabelled pod slivers. It is one
  // labelled block that says what it is waiting for.
  it("draws a channel still downloading its titles as one labelled block", async () => {
    stubGuide();
    const pod = {
      entries: [{ name: "Fizzbolt", role: "commercial" as const, durationMs: 60_000, isFallbackCard: false }],
      matchLevel: "exact" as const,
      totalMs: 60_000,
    };
    server.use(
      getChannelGuideMockHandler({
        ...GUIDE,
        channels: [
          {
            channelId: "ch-part",
            name: "Late Night",
            number: 43,
            status: "live" as const,
            pendingCount: 9,
            programCount: 3,
            airings: [0, 1, 2].map((i) => ({
              kind: "filler" as const,
              scheduleBlockId: `break-${i}`,
              title: "",
              startMs: NOW + i * 60_000,
              stopMs: NOW + (i + 1) * 60_000,
              pod,
            })),
          },
        ],
      }),
    );
    renderAt(GRID);
    const label = "Commercials until titles arrive · 3 of 12 downloaded";
    expect(await screen.findByRole("button", { name: label })).toBeInTheDocument();
    expect(screen.queryByText("Fizzbolt")).not.toBeInTheDocument();
  });

  // The span picker replaced zoom (#1659): the grid always fits its width, so seeing more or less
  // time means asking the server for a longer or shorter window.
  it("changes how much time the guide shows with the span picker, not zoom", async () => {
    const user = userEvent.setup();
    stubGuide();
    const spans: number[] = [];
    server.use(
      getChannelGuideMockHandler(({ request }) => {
        const params = new URL(request.url).searchParams;
        spans.push(Number(params.get("to")) - Number(params.get("from")));
        return GUIDE;
      }),
    );
    const view = renderAt(GRID);
    expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    expect(view.getByRole("button", { name: "Show 4 hours" })).toHaveAttribute("aria-pressed", "true");
    expect(view.queryByRole("button", { name: /zoom/i })).not.toBeInTheDocument();

    await user.click(view.getByRole("button", { name: "Show 2 hours" }));
    expect(view.getByRole("button", { name: "Show 2 hours" })).toHaveAttribute("aria-pressed", "true");
    expect(view.getByRole("button", { name: "Show 4 hours" })).toHaveAttribute("aria-pressed", "false");
    await vi.waitFor(() => expect(spans).toContain(2 * 3_600_000));
  });

  // #1396: every window change is a new react-query key. Without placeholder data the grid went
  // to channels=[] and the toolbar (gated on channels.length) unmounted until the server
  // answered — the full latency, on exactly the navigations where the server is slowest.
  describe("changing the time window", () => {
    // The first request answers at once; every later one waits on `release()`, so a test can
    // look at the page while the next window is genuinely still in flight.
    const stubSlowNextWindow = () => {
      const froms: number[] = [];
      let release = () => {};
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      server.use(
        getChannelGuideMockHandler(async ({ request }) => {
          const from = Number(new URL(request.url).searchParams.get("from"));
          froms.push(from);
          // The initial window, however many times it is requested (loader + component).
          if (from === froms[0]) return GUIDE;
          await gate;
          const next: GuideOutputBody = {
            fromMs: from,
            toMs: from + 4 * 3_600_000,
            channels: [
              {
                channelId: "ch-next",
                name: "Next Window Channel",
                number: 44,
                status: "live",
                pendingCount: 0,
                airings: [],
              },
            ],
          };
          return next;
        }),
      );
      return { froms, release };
    };

    it("keeps the previous rows and the toolbar mounted, with a pending indicator, until the next window arrives", async () => {
      const user = userEvent.setup();
      stubGuide();
      const { release } = stubSlowNextWindow();
      const view = renderAt(GRID);
      expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
      expect(view.queryByRole("status", { name: /loading guide/i })).not.toBeInTheDocument();

      await user.click(view.getByRole("button", { name: "Forward an hour" }));

      // The new window is pending: the old grid and the controls are still there.
      expect(await view.findByRole("status", { name: /loading guide/i })).toBeInTheDocument();
      expect(view.getByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
      expect(view.getByRole("button", { name: "Forward an hour" })).toBeInTheDocument();
      expect(view.getByRole("button", { name: "NOW" })).toBeInTheDocument();
      expect(view.queryByText("Dead air")).not.toBeInTheDocument();

      release();
      expect(await view.findByText("Next Window Channel")).toBeInTheDocument();
      expect(view.queryByRole("status", { name: /loading guide/i })).not.toBeInTheDocument();
    });

    // TanStack v5 shows placeholder data only while a query is pending, so a NEW window that fails
    // has no data at all. The page used to give itself to the error then, taking the controls (and
    // the way back to a window that works) with it.
    describe("that fails to load", () => {
      // The first windows answer (the opened one and its prefetched neighbours); `fail()` then makes
      // the server error for everything not already cached.
      const stubFailingServer = async () => {
        stubGuide();
        let failing = false;
        const asked: number[] = [];
        server.use(
          http.get("*/v1/guide", ({ request }) => {
            asked.push(Number(new URL(request.url).searchParams.get("from")));
            if (failing) return new HttpResponse(null, { status: 503 });
            return undefined;
          }),
        );
        const view = renderAt(GRID);
        expect(
          await view.findByRole("button", { name: /actions for saturday cartoons/i }),
        ).toBeInTheDocument();
        // The neighbours (one hour either side, tomorrow) are warmed once the opened window settles.
        await vi.waitFor(() => expect(asked.length).toBeGreaterThanOrEqual(4));
        await vi.waitFor(() => expect(clients.at(-1)?.isFetching()).toBe(0));
        failing = true;
        return view;
      };

      it.each([
        [
          "a span change",
          async (view: Awaited<ReturnType<typeof stubFailingServer>>) =>
            userEvent.click(view.getByRole("button", { name: "Show 12 hours" })),
        ],
        [
          "stepping two hours past the prefetched one",
          async (view: Awaited<ReturnType<typeof stubFailingServer>>) => {
            await userEvent.click(view.getByRole("button", { name: "Forward an hour" }));
            await userEvent.click(view.getByRole("button", { name: "Forward an hour" }));
          },
        ],
        [
          "stepping two hours back past the prefetched one",
          async (view: Awaited<ReturnType<typeof stubFailingServer>>) => {
            await userEvent.click(view.getByRole("button", { name: "Back an hour" }));
            await userEvent.click(view.getByRole("button", { name: "Back an hour" }));
          },
        ],
        [
          "a day two past the prefetched one",
          async (view: Awaited<ReturnType<typeof stubFailingServer>>) =>
            userEvent.selectOptions(view.getByRole("combobox", { name: "Day" }), "3"),
        ],
      ])("keeps the last Guide and its controls under a notice on %s", async (_name, change) => {
        const view = await stubFailingServer();

        await change(view);

        expect(await view.findByText("Couldn't refresh the Guide")).toBeInTheDocument();
        expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
        expect(view.getByRole("button", { name: "NOW" })).toBeInTheDocument();
        expect(view.getByRole("button", { name: "Forward an hour" })).toBeInTheDocument();
        expect(view.getByRole("combobox", { name: "Day" })).toBeInTheDocument();
        // One Try again (the notice's), not the page-wide error's as well.
        expect(view.getAllByRole("button", { name: "Try again" })).toHaveLength(1);
      });

      it("keeps the controls working after a failure, so the viewer can pick another window", async () => {
        const user = userEvent.setup();
        const view = await stubFailingServer();
        await user.click(view.getByRole("button", { name: "Show 12 hours" }));
        expect(await view.findByText("Couldn't refresh the Guide")).toBeInTheDocument();

        server.resetHandlers();
        stubGuide();
        await user.click(view.getByRole("button", { name: "Show 4 hours" }));

        await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
        expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      });

      it("loads the window the viewer asked for when Try again is pressed and the server has recovered", async () => {
        const user = userEvent.setup();
        const view = await stubFailingServer();
        await user.click(view.getByRole("button", { name: "Show 12 hours" }));
        expect(await view.findByText("Couldn't refresh the Guide")).toBeInTheDocument();

        server.resetHandlers();
        stubGuide();
        await user.click(view.getByRole("button", { name: "Try again" }));

        await waitFor(() => expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument());
        expect(view.getByRole("button", { name: "Show 12 hours" })).toHaveAttribute("aria-pressed", "true");
        expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      });

      // A window with no answer has no data, so its retry starts from a clean query: the notice has to
      // be kept by the page, or it unmounts under the focus on its button.
      it("keeps focus on Try again while a failed window retries, and when it fails again", async () => {
        const user = userEvent.setup();
        const view = await stubFailingServer();
        await user.click(view.getByRole("button", { name: "Show 12 hours" }));
        const button = await view.findByRole("button", { name: "Try again" });
        button.focus();

        let answer = () => {};
        const held = new Promise<void>((resolve) => {
          answer = resolve;
        });
        server.use(
          http.get("*/v1/guide", async () => {
            await held;
            return new HttpResponse(null, { status: 503 });
          }),
        );
        await user.click(button);

        expect(await view.findByRole("button", { name: "Trying again…" })).toBe(button);
        expect(button).toHaveFocus();
        expect(view.getByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();

        answer();
        await waitFor(() => expect(button).toHaveAccessibleName("Try again"));
        expect(button).toHaveFocus();
      });

      it("gives the page to the error only when the Guide has never answered", async () => {
        const user = userEvent.setup();
        stubGuide();
        server.use(http.get("*/v1/guide", () => new HttpResponse(null, { status: 503 })));
        const view = renderAt(GRID);

        expect(await view.findByRole("button", { name: "Try again" })).toBeInTheDocument();
        expect(view.queryByRole("button", { name: "NOW" })).not.toBeInTheDocument();
        expect(view.queryByText("Couldn't refresh the Guide")).not.toBeInTheDocument();

        server.resetHandlers();
        stubGuide();
        await user.click(view.getByRole("button", { name: "Try again" }));
        expect(await view.findByRole("button", { name: /^Space Western,/ })).toBeInTheDocument();
      });
    });

    it("prefetches the adjacent windows once the current one has loaded", async () => {
      stubGuide();
      const { froms, release } = stubSlowNextWindow();
      const view = renderAt(GRID);
      expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

      const HOUR = 3_600_000;
      const current = froms[0] ?? 0;
      // ±1 hour (the stepper) and tomorrow at the same offset (the day picker's next stop).
      await vi.waitFor(() => {
        expect(froms).toEqual(expect.arrayContaining([current - HOUR, current + HOUR]));
        expect(froms.length).toBeGreaterThanOrEqual(4);
      });
      release();
    });
  });

  it("keeps everyday time controls visible and precise view controls behind one disclosure", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);

    // Wait for a guide row so the loading state cannot be mistaken for a closed disclosure.
    expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    expect(view.getByRole("button", { name: "NOW" })).toBeInTheDocument();
    expect(view.getByRole("button", { name: "Show 4 hours" })).toHaveAttribute("aria-pressed", "true");

    // Planning controls do not compete with the everyday toolbar until asked for.
    expect(view.queryByLabelText("Start hour")).not.toBeInTheDocument();

    const viewTrigger = view.getByRole("button", { name: "View options" });
    expect(viewTrigger).toHaveAttribute("aria-expanded", "false");
    await user.click(viewTrigger);

    await user.selectOptions(view.getByRole("combobox", { name: "Start hour" }), "9 PM");
    expect(viewTrigger).toHaveAccessibleName("View options, custom");

    // Closing the row keeps its non-default state legible on the trigger.
    await user.click(viewTrigger);
    expect(view.queryByLabelText("Start hour")).not.toBeInTheDocument();
    expect(view.getByRole("button", { name: "View options, custom" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  // Home's Tonight rows link to a programme that hasn't started (#2763): the grid opens on its day
  // and hour, flagged as a custom view.
  it("opens on the day and hour a link's ?at= names", async () => {
    stubGuide();
    const today = new Date();
    const tomorrowMidnight = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1).getTime();
    const at = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1, 21, 30).getTime();
    const froms: number[] = [];
    server.use(
      http.get("*/v1/guide", ({ request }) => {
        froms.push(Number(new URL(request.url).searchParams.get("from")));
        return undefined;
      }),
    );
    const view = renderAt(`${GRID}&at=${at}&channel=ch-live`);

    expect(await view.findByRole("button", { name: "View options, custom" })).toBeInTheDocument();
    expect(froms).toContain(new Date(tomorrowMidnight).setHours(21));
  });

  // The axis reads in the household's zone, so the hour a link opens on is that zone's, whatever
  // zone the device is in. Kiritimati (UTC+14) is no test machine's zone.
  it("opens a link's ?at= on the household's hour, not the device's", async () => {
    stubGuide();
    const zone = "Pacific/Kiritimati";
    const today = new Date();
    // 07:30 UTC tomorrow is 21:30 in Kiritimati, so the grid opens at its 21:00: 07:00 UTC.
    const at = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() + 1, 7, 30);
    const froms: number[] = [];
    server.use(
      http.get("*/v1/guide", ({ request }) => {
        froms.push(Number(new URL(request.url).searchParams.get("from")));
        return HttpResponse.json({ ...GUIDE, timezone: zone });
      }),
    );
    const view = renderAt(`${GRID}&at=${at}&channel=ch-live`);

    expect(await view.findByRole("button", { name: "View options, custom" })).toBeInTheDocument();
    await waitFor(() => expect(froms).toContain(at - 30 * 60_000));
  });

  it("opens the channel's page from its name, not the player (#2745)", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);
    await view.findByRole("button", { name: /actions for saturday cartoons/i });

    await user.click(view.getByRole("button", { name: /^\d+\s*Saturday Cartoons$/ }));

    await waitFor(() => expect(view.router.state.location.pathname).toMatch(/^\/channels\//));
  });

  it("closes the View panel on Escape (#2745)", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);
    expect(await view.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    const viewTrigger = view.getByRole("button", { name: "View options" });
    await user.click(viewTrigger);
    expect(view.getByLabelText("Start hour")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(view.queryByLabelText("Start hour")).not.toBeInTheDocument();
    expect(viewTrigger).toHaveAttribute("aria-expanded", "false");
  });

  it("opens contextual details on intent and keeps them through pointer transfer until Escape", async () => {
    const user = userEvent.setup();
    stubGuide();
    const now = Date.now();
    const HALF_HOUR = 1_800_000;
    server.use(
      getChannelGuideMockHandler({
        fromMs: now - 2 * HALF_HOUR,
        toMs: now + 6 * HALF_HOUR,
        channels: [
          {
            channelId: "ch-live",
            name: "Saturday Cartoons",
            number: 42,
            status: "live",
            pendingCount: 0,
            airings: [
              {
                kind: "program",
                scheduleBlockId: "block_on_now",
                title: "Space Western",
                startMs: now - HALF_HOUR,
                stopMs: now + HALF_HOUR,
                runtimeMs: 2 * HALF_HOUR,
              },
              {
                kind: "program",
                scheduleBlockId: "block_next",
                title: "Harbour Mystery",
                startMs: now + HALF_HOUR,
                stopMs: now + 3 * HALF_HOUR,
                runtimeMs: 2 * HALF_HOUR,
              },
            ],
          },
        ],
      }),
    );
    const view = renderAt(GRID);

    const next = await view.findByRole("button", { name: /^Harbour Mystery,/ });
    expect(within(next).getByText("Harbour Mystery")).toBeInTheDocument();
    expect(view.queryByRole("tooltip")).not.toBeInTheDocument();
    vi.spyOn(next, "getBoundingClientRect").mockReturnValue(new DOMRect(300, 200, 150, 44));
    await user.hover(next);
    expect(view.queryByRole("tooltip")).not.toBeInTheDocument();
    const preview = await view.findByRole("tooltip");
    expect(within(preview).getByText("Scheduled")).toBeInTheDocument();
    expect(within(preview).getByText("Harbour Mystery")).toBeInTheDocument();
    await user.hover(preview);
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(preview).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(view.queryByRole("tooltip")).not.toBeInTheDocument());
    expect(within(next).getByText("Harbour Mystery")).toBeInTheDocument();
    expect(view.queryByText("On now")).not.toBeInTheDocument();
  });

  // Today's channel row menu fills the grid's ⋯ slot until the maintainer mocks its contents.
  it("puts each channel's actions menu in its row's ⋯ slot", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);

    await user.click(await view.findByRole("button", { name: /actions for saturday cartoons/i }));
    expect(await screen.findByRole("menuitem", { name: /pause/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /delete/i })).toBeInTheDocument();
  });

  it("has no manual rebuild/refresh — edits are seamless (§9) — and each row opens its channel", async () => {
    stubGuide();
    renderAt(GRID);

    // The row is present (its ⋮ actions menu is the stable, explicitly-labelled handle on
    // it — the channel button's accessible name is assembled from sibling spans and is a
    // rendering detail). Awaited because rows depend on the guide query resolving.
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    // No manual "Rebuild"/"Refresh" buttons — a background reconcile + a `channel` SSE
    // frame keep the surface current on their own.
    expect(screen.queryByRole("button", { name: /rebuild/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /refresh/i })).not.toBeInTheDocument();
  });

  it("removes a deleted channel from the current Guide without a page refresh", async () => {
    const user = userEvent.setup();
    let deleted = false;
    stubGuide();
    server.use(
      getChannelGuideMockHandler(() => (deleted ? { ...GUIDE, channels: [] } : GUIDE)),
      getDeleteChannelMockHandler(() => {
        deleted = true;
        return undefined as never;
      }),
    );
    renderAt(GRID);

    await user.click(await screen.findByRole("button", { name: /actions for saturday cartoons/i }));
    await user.click(await screen.findByRole("menuitem", { name: /delete/i }));
    // The armed confirmation's Delete is a menu item too, inside the group that asks the question.
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));

    expect(await screen.findByText("Dead air")).toBeInTheDocument();
    expect(screen.queryByText("Saturday Cartoons")).not.toBeInTheDocument();
  });

  it("is the door to origination: 'Add a channel' goes to the request page (#2746)", async () => {
    const user = userEvent.setup();
    stubGuide();
    const view = renderAt(GRID);

    // Each row carries a ⋮ actions menu (pause/resume + delete) so removing a channel doesn't
    // require opening it — awaited because the rows depend on the guide query resolving.
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    // THE origination door (§12): describing a channel happens on its own page under Requests,
    // at a plain path with no params.
    const add = screen.getByRole("link", { name: /add a channel/i });
    expect(add).toHaveAttribute("href", "/requests/new");
    await user.click(add);
    expect(await screen.findByLabelText("Channel intent")).toBeInTheDocument();
    expect(view.router.state.location.href).toBe("/requests/new");
  });

  // The fresh-install state. ⚠ Before this, the header's "Add a channel" and the empty
  // state's "Describe your first channel" were BOTH on screen, both calling the same
  // handler, opening the same panel — which then titled itself "Add a channel". Three
  // labels, one action, two of them visible at once.
  it("shows only ONE origination door on an empty guide, and names it the same as the header", async () => {
    stubGuide(me(), { empty: true });
    renderAt(GRID);

    expect(await screen.findByText("Dead air")).toBeInTheDocument();
    // Exactly one control offers the action, and it uses the header's own wording.
    const doors = screen.getAllByRole("button", { name: /add a channel/i });
    expect(doors).toHaveLength(1);
    // The old second label is gone entirely.
    expect(screen.queryByRole("button", { name: /describe your first channel/i })).not.toBeInTheDocument();
  });

  // The e2e mock backend answers unknown endpoints with `{}`. The page reads `channels`
  // defensively, and laying out a body without them crashed the whole route (#1707's queue run).
  it("shows Dead air, not an error, when the guide answers without channels", async () => {
    stubGuide(me(), { empty: true });
    server.use(http.get("*/v1/guide", () => HttpResponse.json({})));
    renderAt(GRID);

    expect(await screen.findByText("Dead air")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Guide", level: 1 })).toBeInTheDocument();
  });

  // The empty state's one door goes to the same page as the header's.
  it("sends the empty state's door to the request page", async () => {
    const user = userEvent.setup();
    stubGuide(me(), { empty: true });
    const view = renderAt(GRID);

    await view.findByText("Dead air");
    await user.click(view.getByRole("button", { name: /add a channel/i }));

    expect(await view.findByLabelText("Channel intent")).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe("/requests/new");
  });

  it("keeps the header door on a populated guide, where no empty state offers it", async () => {
    stubGuide();
    renderAt(GRID);

    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /add a channel/i })).toBeInTheDocument();
    expect(screen.queryByText("Dead air")).not.toBeInTheDocument();
  });

  it("names the origination door for a member — they request rather than add", async () => {
    stubGuide(MEMBER);
    renderAt(GRID);
    // A member has no other way to ask for a channel now that /suggest is gone, so the
    // affordance must be present for them too — worded for what they are actually doing.
    expect(await screen.findByRole("link", { name: /request a channel/i })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /add a channel/i })).not.toBeInTheDocument();
  });
});

// #2746: the flow lived on the Guide route, as `/guide?new=%221%22`. Those URLs still arrive from
// old links and bookmarks; they land on the request page, whose URL reads plainly.
describe("old Guide origination URLs", () => {
  it("sends ?new=1 to the empty describe form on /requests/new", async () => {
    stubGuide();
    const view = renderAt("/guide?new=%221%22");

    expect(await screen.findByLabelText("Channel intent")).toHaveValue("");
    expect(view.router.state.location.href).toBe("/requests/new");
  });

  // §13's blank-page killer: the wizard's handoff lands on a FILLED form.
  it("carries ?intent= over to the request page's form", async () => {
    stubGuide();
    const view = renderAt("/guide?intent=saturday-morning%20cartoons");

    expect(await screen.findByLabelText("Channel intent")).toHaveValue("saturday-morning cartoons");
    expect(view.router.state.location.pathname).toBe("/requests/new");
  });

  it("carries ?job= and ?manual=1 over, with the flag as a plain 1", async () => {
    stubGuide();
    server.use(
      getGetProposalJobMockHandler({
        version: 1,
        jobId: "job-1",
        milestone: "generating",
        intent: { description: "90s action movies" },
        attempts: [{ version: 1, number: 1, status: "running", startedAt: "2026-08-22T12:00:00Z" }],
        actions: ["wait"],
        createdAt: "2026-08-22T12:00:00Z",
        updatedAt: "2026-08-22T12:00:00Z",
      }),
    );
    const view = renderAt("/guide?job=job-1&manual=1");

    await waitFor(() => expect(view.router.state.location.pathname).toBe("/requests/new"));
    expect(view.router.state.location.href).toBe("/requests/new?job=job-1&manual=1");
  });

  it("leaves an ordinary Guide visit on the Guide", async () => {
    stubGuide();
    const view = renderAt("/guide");

    expect(await screen.findByRole("heading", { name: "Guide", level: 1 })).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe("/guide");
  });
});
