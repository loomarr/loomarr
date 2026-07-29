import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// The Guide — the channels surface — through the REAL route tree.
//
// Was `channels-board.test.tsx` against `/channels`. That route folded into `/guide` (§12):
// one surface answers "what do I have" and "what is on", and it owns ORIGINATION. The
// assertions that survived the move are the ones about behavior rather than layout — the
// per-row actions menu, the inline "Add a channel" door, and the absence of any manual
// rebuild control (§9 self-maintaining). The card-list-shaped ones did not survive, because
// the cards did not.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const MEMBER = { id: "u2", name: "Kid", role: "member", autoApprove: false, disabled: false, quota: 0 };

const CHANNELS = [
  {
    id: "ch-live",
    name: "Saturday Cartoons",
    number: 42,
    status: "live",
    strategy: "shuffle",
    programCount: 10,
    pendingCount: 0,
    breakCount: 4,
    slotCount: 14,
    policy: {},
  },
  {
    id: "ch-part",
    name: "Late Night",
    number: 43,
    status: "live",
    strategy: "shuffle",
    programCount: 3,
    pendingCount: 7,
    breakCount: 0,
    slotCount: 10,
    policy: {},
  },
];

const TITLES = [
  { key: "movie:tmdb:1", mediaType: "movie", state: "available", name: "Landed" },
  { key: "movie:tmdb:2", mediaType: "movie", state: "downloading", name: "Coming" },
  { key: "movie:tmdb:3", mediaType: "movie", state: "unavailable", name: "Gave Up", tmdbId: 3 },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

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
      status: "live",
      pendingCount: 0,
      airings: [
        { kind: "program", title: "The Matrix", startMs: NOW, stopMs: NOW + 7_200_000, runtimeMs: 7_200_000 },
      ],
    },
  ],
};

// `empty` serves a guide with NO channels, which is the fresh-install state the "Dead air"
// empty state and the hidden header door both hang off.
const stubFetch = (me: unknown = ADMIN, opts: { empty?: boolean } = {}) => {
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(me));
    if (u.includes("/v1/guide"))
      return Promise.resolve(json(opts.empty ? { ...GUIDE, channels: [] } : GUIDE));
    if (u.includes("/v1/channels")) return Promise.resolve(json({ channels: opts.empty ? [] : CHANNELS }));
    // GET /v1/titles is a single-state FILTER, and it 400s without a `state` — mirror the
    // real handler (internal/api/titles.go) rather than answering any URL, so a caller
    // that forgets the param fails here exactly as it would in production. Returning the
    // full set for every request is what let the Board ship a param-less call that 400s
    // live while the suite stayed green.
    if (u.includes("/v1/titles") && (!_init || _init.method === undefined || _init.method === "GET")) {
      const state = new URL(u, "http://x").searchParams.get("state");
      if (!state)
        return Promise.resolve(json({ title: "Bad Request", detail: "state query param is required" }, 400));
      return Promise.resolve(json({ titles: TITLES.filter((t) => t.state === state) }));
    }
    if (u.includes("/v1/settings")) {
      // tunarr.url set → the list's Rebuild button is enabled (it's gated on Tunarr being
      // connected, so a rebuild can't 501). Minimal entry shape the useTunarrReady hook reads.
      return Promise.resolve(
        json({
          features: {},
          settings: [{ key: "tunarr.url", set: true, provenance: "db", secret: false }],
        }),
      );
    }
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  return mock;
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  // Returns the render RESULT so a test can scope its queries to its own tree. `screen`
  // searches the shared document.body, and a query that reaches a neighbouring test's markup
  // clicks a detached button, which silently does nothing.
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

describe("Guide", () => {
  it("is headed 'Channels' and shows what's on, from the guide endpoint", async () => {
    stubFetch();
    renderAt("/guide");
    // The heading is "Channels", not "Guide": one surface, and the mock names it for the
    // objects it lists rather than the view it uses (§12).
    expect(await screen.findByRole("heading", { name: "Channels", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("Saturday Cartoons")).toBeInTheDocument();
    expect(await screen.findByText(/The Matrix/)).toBeInTheDocument();
  });

  it("has no manual rebuild/refresh — edits are seamless (§9) — and each row opens its channel", async () => {
    stubFetch();
    renderAt("/guide");

    // The row is present (its ⋮ actions menu is the stable, explicitly-labelled handle on
    // it — the channel button's accessible name is assembled from sibling spans and is a
    // rendering detail). Awaited because rows depend on the guide query resolving.
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    // No manual "Rebuild"/"Refresh" buttons — a background reconcile + a `channel` SSE
    // frame keep the surface current on their own.
    expect(screen.queryByRole("button", { name: /rebuild/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /refresh/i })).not.toBeInTheDocument();
  });

  it("owns origination: 'Add a channel' opens the describe panel in place", async () => {
    const user = userEvent.setup();
    stubFetch();
    renderAt("/guide");

    // Each row carries a ⋮ actions menu (pause/resume + delete) so removing a channel doesn't
    // require opening it — awaited because the rows depend on the guide query resolving.
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();

    // THE origination door (§12), and the reason `/channels` could fold away: describing a
    // channel happens here, inline, rather than on a separate page.
    const add = screen.getByRole("button", { name: /add a channel/i });
    await user.click(add);
    expect(await screen.findByLabelText("Channel intent")).toBeInTheDocument();
  });

  // The fresh-install state. ⚠ Before this, the header's "Add a channel" and the empty
  // state's "Describe your first channel" were BOTH on screen, both calling the same
  // handler, opening the same panel — which then titled itself "Add a channel". Three
  // labels, one action, two of them visible at once.
  it("shows only ONE origination door on an empty guide, and names it the same as the header", async () => {
    stubFetch(ADMIN, { empty: true });
    renderAt("/guide");

    expect(await screen.findByText("Dead air")).toBeInTheDocument();
    // Exactly one control offers the action, and it uses the header's own wording.
    const doors = screen.getAllByRole("button", { name: /add a channel/i });
    expect(doors).toHaveLength(1);
    // The old second label is gone entirely.
    expect(screen.queryByRole("button", { name: /describe your first channel/i })).not.toBeInTheDocument();
  });

  // ⚠ The dead end this avoids: the header button becomes "Close" once the panel is open,
  // and an empty guide is exactly when someone is most likely to have opened it. Hiding it
  // unconditionally would leave the panel with no way out.
  it("brings the header door back as Close once the panel is open on an empty guide", async () => {
    const user = userEvent.setup();
    stubFetch(ADMIN, { empty: true });
    const view = renderAt("/guide");

    // Wait for the EMPTY STATE, not just the button. "Dead air" only renders once the guide
    // query has answered; before that `channels` is [] for a different reason (still loading)
    // and the header door is still on screen. Clicking that one opens the panel in a tree the
    // very next render replaces, so the Close never appears.
    await view.findByText("Dead air");
    await user.click(view.getByRole("button", { name: /add a channel/i }));

    // Scoped to THIS render rather than `screen`: document.body is shared across tests in
    // the file, and a query that reaches a neighbour's markup clicks a detached button, which
    // silently does nothing. The point of the assertion is only that the EXIT exists; the
    // panel's own contents are covered by the origination test above.
    expect(await view.findByRole("button", { name: /close/i })).toBeInTheDocument();
  });

  it("keeps the header door on a populated guide, where no empty state offers it", async () => {
    stubFetch();
    renderAt("/guide");

    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add a channel/i })).toBeInTheDocument();
    expect(screen.queryByText("Dead air")).not.toBeInTheDocument();
  });

  it("names the origination door for a member — they request rather than add", async () => {
    stubFetch(MEMBER);
    renderAt("/guide");
    // A member has no other way to ask for a channel now that /suggest is gone, so the
    // affordance must be present for them too — worded for what they are actually doing.
    expect(await screen.findByRole("button", { name: /request a channel/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /add a channel/i })).not.toBeInTheDocument();
  });

  it("opens the describe panel when the wizard hands off ?intent=", async () => {
    stubFetch();
    renderAt("/guide?intent=saturday-morning%20cartoons");
    // §13's blank-page killer: the handoff must land on a FILLED form, not a bare grid with
    // the operator wondering where their template went.
    const intent = await screen.findByLabelText("Channel intent");
    expect(intent).toHaveValue("saturday-morning cartoons");
  });
});

describe("Board", () => {
  it("leads with the journey, not a table of states", async () => {
    stubFetch();
    renderAt("/queue");
    // "1 of 3 have landed" — the member framing (§13).
    expect(await screen.findByText(/1 of 3 titles have landed/i)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /on the way/i })).toBeInTheDocument();
  });

  it("offers a retry only for a title that gave up", async () => {
    const fetchMock = stubFetch();
    renderAt("/queue");

    const retries = await screen.findAllByRole("button", { name: /try again/i });
    expect(retries).toHaveLength(1); // only the unavailable one
    await userEvent.click(retries[0] as HTMLElement);

    const posted = fetchMock.mock.calls.find(
      ([u, i]) => String(u).includes("/v1/titles") && String(i?.method) === "POST",
    );
    // Re-enqueued by identity, not by key — that is what the enqueue contract takes.
    expect(JSON.parse(String(posted?.[1]?.body))).toMatchObject({ mediaType: "movie", tmdbId: 3 });
  });
});
