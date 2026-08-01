import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const MEMBER = { ...ADMIN, role: "member" };

const clip = (over: Record<string, unknown> = {}) => ({
  path: "c1.mp4",
  tunarrProgramId: "c1",
  name: "Frosted Flakes",
  kind: "commercial",
  durationMs: 30000,
  era: 1992,
  audience: "kids",
  category: "cereal",
  tagged: true,
  aiTagged: false,
  ...over,
});

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

type Opts = {
  features?: Record<string, boolean>;
  clips?: Array<Record<string, unknown>>;
  me?: Record<string, unknown>;
};

// The SSE stream, captured so a test can fire frames at the app: the split job's terminal
// `filler_split` frame is what hands the review route its proposal id (V34), and a no-op
// EventSource would leave that handoff untestable.
class CaptureEventSource {
  static listeners = new Map<string, Array<(ev: MessageEvent) => void>>();
  addEventListener(type: string, cb: (ev: MessageEvent) => void) {
    const list = CaptureEventSource.listeners.get(type) ?? [];
    list.push(cb);
    CaptureEventSource.listeners.set(type, list);
  }
  close() {}
}

const fireFrame = (type: string, payload: unknown) => {
  const data = JSON.stringify(payload);
  for (const cb of CaptureEventSource.listeners.get(type) ?? []) {
    cb({ data } as MessageEvent);
  }
};

const stubFetch = ({
  features = { filler: true, suggestions: true },
  clips = [clip()],
  me = ADMIN,
}: Opts = {}) => {
  CaptureEventSource.listeners = new Map();
  const mock = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(me));
    if (u.includes("/v1/filler/sync"))
      return Promise.resolve(json({ total: 3, added: 2, updated: 1, pruned: 0 }));
    if (u.includes("/v1/filler/tag"))
      return Promise.resolve(json({ considered: 2, tagged: 2, partial: 0, skipped: 0 }));
    if (u.includes("/v1/filler/ingest")) return Promise.resolve(json({ jobId: "job-1" }));
    // Split (V34): detection starts as a job; the review route reads the proposal back.
    if (u.endsWith("/split")) return Promise.resolve(json({ jobId: "job-split-1" }));
    if (u.includes("/v1/filler/splits/")) {
      return Promise.resolve(
        json({
          id: "sp-1",
          clipPath: "c1.mp4",
          createdAt: "2026-07-25T20:00:00Z",
          segments: [{ index: 0, startMs: 0, endMs: 30000, name: "First ad" }],
        }),
      );
    }
    if (u.includes("/v1/filler/") && String(init?.method) === "PATCH") {
      return Promise.resolve(json(clips[0]));
    }
    if (u.includes("/v1/filler")) {
      // Honor the query string so a filter test proves the SERVER did the filtering.
      const query = u.includes("?") ? u.slice(u.indexOf("?")) : "";
      const params = new URLSearchParams(query);
      let out = clips;
      const q = params.get("q");
      if (q) out = out.filter((c) => String(c.name).toLowerCase().includes(q.toLowerCase()));
      if (params.get("kind")) out = out.filter((c) => c.kind === params.get("kind"));
      return Promise.resolve(json({ clips: out }));
    }
    if (u.includes("/v1/settings")) return Promise.resolve(json({ features, settings: [] }));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal("EventSource", CaptureEventSource);
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
  return router;
};

afterEach(() => vi.restoreAllMocks());

describe("Filler page", () => {
  it("lists the catalog with each clip's match tags", async () => {
    stubFetch();
    renderAt("/filler");
    expect(await screen.findByText("Frosted Flakes")).toBeInTheDocument();
    expect(screen.getByText("1992s")).toBeInTheDocument();
  });

  // Search is executed SERVER-side (§7.2 name LIKE) rather than filtering in memory —
  // the store already indexes these columns and the catalog can run to thousands.
  it("sends the search term to the server", async () => {
    const fetchMock = stubFetch({
      clips: [
        clip(),
        clip({ path: "c2.mp4", tunarrProgramId: "c2", name: "TMNT figures", category: "toys" }),
      ],
    });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    await userEvent.type(screen.getByLabelText("Search"), "tmnt");

    await screen.findByText("TMNT figures");
    expect(screen.queryByText("Frosted Flakes")).not.toBeInTheDocument();
    const searched = fetchMock.mock.calls.some(([u]) => String(u).includes("q=tmnt"));
    expect(searched, "the filter must reach the API, not filter a cached list").toBe(true);
  });

  it("runs a catalog sync and reports what changed", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    await userEvent.click(screen.getByRole("button", { name: /^sync$/i }));
    expect(await screen.findByText(/2 added, 1 updated, 0 pruned/i)).toBeInTheDocument();
  });

  // AI tagging needs the same LLM the Suggest tab uses; without it the call 409s, so the
  // button must be disabled rather than offered and rejected.
  it("disables AI tagging when no LLM is configured", async () => {
    stubFetch({ features: { filler: true, suggestions: false } });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    expect(screen.getByRole("button", { name: /ai tag/i })).toBeDisabled();
  });

  it("explains rather than listing when no filler folder is configured", async () => {
    stubFetch({ features: { filler: false } });
    renderAt("/filler");
    expect(await screen.findByText(/no filler folder configured/i)).toBeInTheDocument();
  });

  // The ingest gate is the one no setting can open: it depends on the running IMAGE.
  // Pointing the operator at Settings would be a dead end, so the copy names the image.
  // ⚠ Both ingest tests render the DISCOVER tab: the panel moved there when Discover
  // became a peer of Catalog rather than a card stacked under it.
  it("names the image, not a setting, when ingest tooling is absent", async () => {
    stubFetch({ features: { filler: true, ingest: false } });
    renderAt("/filler?tab=discover");
    expect(await screen.findByText(/loomarr:filler/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^download$/i })).not.toBeInTheDocument();
  });

  it("starts an ingest job when the tooling is present", async () => {
    const fetchMock = stubFetch({ features: { filler: true, ingest: true } });
    renderAt("/filler?tab=discover");
    await screen.findByLabelText("URLs");

    await userEvent.type(screen.getByLabelText("URLs"), "https://archive.org/details/classic-tv-commercials");
    await userEvent.click(screen.getByRole("button", { name: /^download$/i }));

    const posted = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/filler/ingest"));
    expect(posted, "Download should POST the URLs").toBeDefined();
    expect(JSON.parse(String(posted?.[1]?.body)).urls).toEqual([
      "https://archive.org/details/classic-tv-commercials",
    ]);
  });

  it("opens the tag editor and saves a corrected kind", async () => {
    const fetchMock = stubFetch({ clips: [clip({ kind: "commercial", name: "Some Trailer" })] });
    renderAt("/filler");
    await screen.findByText("Some Trailer");

    await userEvent.click(screen.getByRole("button", { name: /edit tags/i }));

    // Scoped to the editor's region: the page's own Kind/Audience FILTERS share those
    // visible names, which is why the editor is a labelled region in the first place.
    // Open the editor's Kind select, then pick "trailer" — the listbox portals to the
    // body (outside the region), so its option is found at the screen level.
    const editor = await screen.findByRole("region", { name: /edit tags: some trailer/i });
    await userEvent.click(within(editor).getByLabelText("Kind"));
    await userEvent.click(await screen.findByRole("option", { name: "Trailer" }));
    await userEvent.click(within(editor).getByRole("button", { name: /save tags/i }));

    const patch = fetchMock.mock.calls.find(([, i]) => String(i?.method) === "PATCH");
    expect(patch, "saving tags should PATCH the clip").toBeDefined();
    expect(JSON.parse(String(patch?.[1]?.body)).kind).toBe("trailer");
  });

  // §10 era grounding (V34): the ungrounded AI year is a QUESTION on the card, and the
  // admin's one-click confirm PATCHes it — carrying the clip's existing audience/category,
  // because the BE's UpdateClipTags writes all three columns unconditionally and a bare
  // {era} would wipe the other two.
  it("confirms an era suggestion, keeping the clip's other tags", async () => {
    const fetchMock = stubFetch({
      clips: [clip({ era: 0, suggestedEra: 1985, audience: "kids", category: "cereal", tagged: false })],
    });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    expect(screen.getByText("1985s?")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /confirm 1985/i }));

    const patch = fetchMock.mock.calls.find(([, i]) => String(i?.method) === "PATCH");
    expect(patch, "the confirm should PATCH the clip").toBeDefined();
    expect(JSON.parse(String(patch?.[1]?.body))).toEqual({ era: 1985, audience: "kids", category: "cereal" });
  });

  // A member sees the suggestion but NOT its answer — the PATCH is admin-only server-side
  // (§19), and the UI gate is the courtesy that keeps the console clean.
  it("shows a member the era question without the confirm action", async () => {
    stubFetch({ me: MEMBER, clips: [clip({ era: 0, suggestedEra: 1985, tagged: false })] });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    expect(screen.getByText("1985s?")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /confirm 1985/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /split into clips/i })).not.toBeInTheDocument();
  });

  // The V34 handoff: POST returns a job id, the terminal filler_split frame carries the
  // proposal id, and the app navigates to the review gate, which reads the proposal back.
  it("starts split detection and navigates to the review on the success frame", async () => {
    const fetchMock = stubFetch();
    const router = renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    await userEvent.click(screen.getByRole("button", { name: /split into clips/i }));
    const posted = fetchMock.mock.calls.find(
      ([u, i]) => String(u).endsWith("/split") && String(i?.method) === "POST",
    );
    expect(posted, "the action should POST the split job").toBeDefined();
    expect(await screen.findByText(/detecting cuts in c1\.mp4/i)).toBeInTheDocument();

    act(() => {
      fireFrame("filler_split", { jobId: "job-split-1", clipPath: "c1.mp4", status: "running" });
    });
    act(() => {
      fireFrame("filler_split", {
        jobId: "job-split-1",
        clipPath: "c1.mp4",
        status: "success",
        proposalId: "sp-1",
        segments: 1,
      });
    });

    expect(await screen.findByRole("heading", { name: /review split/i })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/filler/splits/sp-1");
  });

  it("surfaces the split job's terminal error instead of navigating", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    await userEvent.click(screen.getByRole("button", { name: /split into clips/i }));

    act(() => {
      fireFrame("filler_split", {
        jobId: "job-split-1",
        clipPath: "c1.mp4",
        status: "error",
        error: "ffprobe found no streams",
      });
    });

    expect(await screen.findByText("ffprobe found no streams")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /review split/i })).not.toBeInTheDocument();
  });
});
