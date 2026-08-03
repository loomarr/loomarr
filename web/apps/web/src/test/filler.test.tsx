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
  // Overrides the header pill's payload. Needed because `clips` here seeds the CATALOG, and the
  // states worth testing in the header are the ones where the catalog and the review queue
  // disagree — a fresh auto-fetch has zero of one and a dozen of the other.
  watch?: Record<string, unknown>;
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
  watch,
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
    // V35 reads. Stubbed BEFORE the catch-all `/v1/filler` branch below, which would otherwise
    // answer these with a clip list — a wrong shape that renders as an empty strip rather than
    // an error, i.e. exactly the kind of silence a test should not have to chase.
    if (u.includes("/v1/filler/pool")) {
      return Promise.resolve(
        json({
          clips: clips.length,
          commercials: clips.length,
          eligible: clips.length,
          untagged: 0,
          channels: [],
        }),
      );
    }
    if (u.includes("/v1/filler/incoming")) {
      return Promise.resolve(json({ asks: [], reels: [], total: 0 }));
    }
    // The header pill's live status (§10 V38c). ⚠ Served here because the header reads it from
    // the SERVER — counts and health verdict both — rather than deriving them from the sources
    // list, which is admin-only and would leave a member's pill permanently grey.
    if (u.includes("/v1/filler/watch")) {
      return Promise.resolve(
        json(
          watch ?? {
            health: "healthy",
            sourcesOn: 1,
            sourcesTotal: 2,
            clips: clips.length,
            held: 0,
          },
        ),
      );
    }
    if (u.includes("/v1/filler/bulk/")) return Promise.resolve(json({ updated: 1, missing: 0 }));
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

  // ⚠ The whole-catalog Sync and AI-tag buttons are GONE (V38c, maintainer). Two tests here
  // pinned them — "runs a catalog sync and reports what changed" and "disables AI tagging when
  // no LLM is configured" — and both were removed rather than repaired, because they asserted
  // affordances that no longer exist rather than behaviour that broke.
  //
  // Neither was work an operator should have to remember to start: the sync runs on its own
  // schedule and drains the watch folder every pass, and tagging follows it. A button for
  // something that already happens invites the reading that it does NOT happen unless pressed.
  // This test is what stops them being re-added by reflex.
  it("offers no whole-catalog sync or tag button — that work runs on its own", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    expect(screen.queryByRole("button", { name: /^sync$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /ai tag/i })).not.toBeInTheDocument();
  });

  // The mock's `watchLine`: what is on, what is held, when anything last arrived. ⚠ "N of M
  // sources on" rather than a bare count — an operator who switched a source off wants to see
  // that, and "3 sources" on an install where one is dark is a reassuring lie.
  it("heads the page with how many sources are on", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    expect(await screen.findByText(/\d+ of \d+ sources on/i)).toBeInTheDocument();
  });

  // ⚠ **A held clip must not read as a missing clip.** Auto-fetch holds everything it downloads
  // until someone reviews it, so the FIRST successful fetch leaves the catalog at zero and the
  // Incoming queue full. The header said "5 of 5 sources on · 0 clips" on an install that had just
  // pulled twelve — a working fetcher rendered as a broken one, which is how the maintainer came
  // to ask why the Catalog was empty.
  //
  // The two counts stay SEPARATE clauses. Summing them would be the opposite lie: it would claim a
  // channel can play clips nobody has approved yet.
  it("says how many clips are waiting rather than reporting an empty catalog", async () => {
    stubFetch({
      clips: [],
      watch: { health: "healthy", sourcesOn: 5, sourcesTotal: 5, clips: 0, held: 12 },
    });
    renderAt("/filler");

    expect(await screen.findByText(/0 clips · 12 waiting/i)).toBeInTheDocument();
  });

  // The mirror: with nothing held the clause is absent entirely, not "0 waiting". A permanent
  // zero is noise on the many installs that never hold anything, and noise in a status line is
  // how an operator learns to stop reading it.
  it("omits the waiting clause when nothing is held", async () => {
    stubFetch({
      watch: { health: "healthy", sourcesOn: 5, sourcesTotal: 5, clips: 9, held: 0 },
    });
    renderAt("/filler");

    await screen.findByText(/9 clips/i);
    expect(screen.queryByText(/waiting/i)).not.toBeInTheDocument();
  });

  it("explains rather than listing when no filler folder is configured", async () => {
    stubFetch({ features: { filler: false } });
    renderAt("/filler");
    expect(await screen.findByText(/no filler folder configured/i)).toBeInTheDocument();
  });

  // ⚠ TWO ingest tests were here and are DELETED with the panel they drove (V38b, retired-ok):
  // one asserted the degraded-install copy, the other typed a URL and clicked Download.
  //
  // They are not replaced, because the behaviour is not relocated — it is gone. Clips arrive
  // because you added a SOURCE: registration, per-row search, an approved pull, or the auto-fetch
  // job, each of which has its own coverage. A test kept alive against a deleted surface is worse
  // than no test: it passes by rendering something nobody can reach.
  //
  // ⚠ The `loomarr:filler` (retired-ok) absence assertion went with them. That check now lives
  // where it belongs — `scripts/check-retired.sh` greps the whole tree for the dead image name,
  // which is stronger than one component test asserting one string is missing from one panel.

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

  // ⚠ THE FOOTGUN, pinned at the page level. `UpdateClipTags` overwrites era, audience AND
  // category on every call, so a cycle that PATCHed only the clicked field would silently
  // wipe the other two — once per click, not once per dialog. This asserts the whole tag row
  // travels with a single cycled chip.
  it("sends the clip's other tags when one is cycled, so none are wiped", async () => {
    const fetchMock = stubFetch({
      clips: [clip({ era: 1990, audience: "kids", category: "cereal", tagged: true })],
    });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    await userEvent.click(screen.getByRole("button", { name: /change the audience/i }));

    const patch = fetchMock.mock.calls.find(([, i]) => String(i?.method) === "PATCH");
    expect(patch, "cycling a chip should PATCH the clip").toBeDefined();
    // era and category ride along UNCHANGED; only audience advances.
    expect(JSON.parse(String(patch?.[1]?.body))).toEqual({
      era: 1990,
      audience: "family",
      category: "cereal",
    });
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

  // --- bulk selection (V35) ---

  it("shows the bulk bar only once something is selected", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    expect(screen.queryByText(/clip selected/i)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("checkbox", { name: /select frosted flakes/i }));

    expect(await screen.findByText("1 clip selected")).toBeInTheDocument();
  });

  // ⚠ The copy is a promise. The server's action is a TOMBSTONE — the clip leaves the catalog
  // and stops being used in breaks, and the file stays where the operator put it. A label
  // saying "Delete" would describe something Loomarr deliberately does not do.
  it("offers removal in terms of the catalog, never the files", async () => {
    stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    await userEvent.click(screen.getByRole("checkbox", { name: /select frosted flakes/i }));

    const remove = await screen.findByRole("button", { name: /remove from catalog/i });
    expect(remove).toHaveAttribute("title", expect.stringMatching(/files stay/i));
    expect(screen.queryByRole("button", { name: /^delete/i })).not.toBeInTheDocument();
  });

  it("bulk-removes the selection through the bulk route", async () => {
    const mock = stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    await userEvent.click(screen.getByRole("checkbox", { name: /select frosted flakes/i }));
    await userEvent.click(await screen.findByRole("button", { name: /remove from catalog/i }));

    const call = mock.mock.calls.find(([url]) => String(url).includes("/v1/filler/bulk/remove"));
    expect(call).toBeDefined();
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ paths: ["c1.mp4"] });
  });

  // ⚠ Each dropdown sends ONLY its own field. A bar that posted all three would blank whatever
  // the operator had not touched — the failure the server's per-field optionality prevents, and
  // which only holds if the client sends a partial body.
  it("sends only the tag field the operator picked", async () => {
    const mock = stubFetch();
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");
    await userEvent.click(screen.getByRole("checkbox", { name: /select frosted flakes/i }));

    // ⚠ "Set audience", not "Audience": the catalog's filter bar already owns that name, and
    // the first draft of this test found two comboboxes. The distinct label is the fix, and
    // this assertion is what keeps them distinct.
    await userEvent.click(await screen.findByRole("combobox", { name: "Set audience" }));
    await userEvent.click(await screen.findByRole("option", { name: "family" }));

    const call = mock.mock.calls.find(([url]) => String(url).includes("/v1/filler/bulk/tag"));
    expect(call).toBeDefined();
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ paths: ["c1.mp4"], audience: "family" });
  });

  // A member cannot bulk-edit, so the control that would 403 is simply absent rather than
  // present-and-failing.
  it("gives a member no way to select clips", async () => {
    stubFetch({ me: MEMBER });
    renderAt("/filler");
    await screen.findByText("Frosted Flakes");

    expect(screen.queryByRole("checkbox", { name: /select frosted flakes/i })).not.toBeInTheDocument();
  });
});
