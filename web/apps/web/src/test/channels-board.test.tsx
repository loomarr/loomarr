import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// Channels + Board through the REAL route tree (13.4b).
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

const CHANNELS = [
  {
    id: "ch-live",
    name: "Saturday Cartoons",
    number: 42,
    status: "live",
    strategy: "shuffle",
    programCount: 10,
    slotCount: 10,
    policy: {},
  },
  {
    id: "ch-part",
    name: "Late Night",
    number: 43,
    status: "live",
    strategy: "shuffle",
    programCount: 3,
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

const stubFetch = () => {
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/channels/now-next")) {
      return Promise.resolve(
        json({
          channels: [
            { channelId: "ch-live", now: { title: "The Matrix", startMs: 1, stopMs: 2, gap: false } },
          ],
        }),
      );
    }
    if (u.includes("/v1/channels")) return Promise.resolve(json({ channels: CHANNELS }));
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
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

describe("Channels", () => {
  it("shows what's on now, from the guide", async () => {
    stubFetch();
    renderAt("/channels");
    expect(await screen.findByText("Saturday Cartoons")).toBeInTheDocument();
    // now/next comes from Tunarr's guide via the one-call endpoint.
    expect(await screen.findByText(/The Matrix/)).toBeInTheDocument();
  });

  it("has no manual rebuild/refresh — edits are seamless (§9), the card links to the channel", async () => {
    stubFetch();
    renderAt("/channels");

    // The card is present and links to the channel's page (where editing + refine live).
    const card = await screen.findByRole("link", { name: /Cartoons/i });
    expect(card).toHaveAttribute("href", expect.stringContaining("/channels/ch-live"));

    // No manual "Rebuild"/"Refresh" buttons — a background reconcile + a `channel` SSE
    // frame keep the list current on their own.
    expect(screen.queryByRole("button", { name: /rebuild/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /refresh/i })).not.toBeInTheDocument();
  });

  it("offers the admin add/remove affordances — prominent Suggest, New channel, and a per-row menu", async () => {
    stubFetch();
    renderAt("/channels");

    // Each row carries a ⋮ actions menu (pause/resume + delete) so removing a channel doesn't
    // require opening it — awaited because the rows depend on the channels query resolving.
    expect(await screen.findByRole("button", { name: /actions for saturday cartoons/i })).toBeInTheDocument();
    // The two header actions: Suggest is the headline path, New channel the hand-made one.
    expect(screen.getByRole("button", { name: /suggest a channel/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /new channel/i })).toBeInTheDocument();
  });
});

describe("Board", () => {
  it("leads with the journey, not a table of states", async () => {
    stubFetch();
    renderAt("/board");
    // "1 of 3 have landed" — the member framing (§13).
    expect(await screen.findByText(/1 of 3 titles have landed/i)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /on the way/i })).toBeInTheDocument();
  });

  it("offers a retry only for a title that gave up", async () => {
    const fetchMock = stubFetch();
    renderAt("/board");

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
