import type { FillerSourceDTO } from "@loomarr/api";
import {
  getAddFillerSourceMockHandler,
  getListFillerSourcesMockHandler,
  getMeMockHandler,
  getSetFillerSourceEnabledMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { SourcesPanel } from "./sources-panel";

// SourcesPanel owns the Sources tab's own state and mutations — the switches, the add form, the
// per-source search. It was extracted out of `filler-page.tsx` when that file reached ~1100 lines
// and every keystroke in the source-search box re-rendered the whole clip grid.
//
// ⚠ `sources`/`total` arrive as PROPS rather than from a query here: the page header's status
// line reads the same list, and a second `useListFillerSources` inside this panel would
// double-fetch it. These tests pass them directly, which is also what makes them fast.

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// Records method + url + parsed body, so a test can prove WHAT was sent rather than only that
// something was.
// ⚠ `/v1/auth/me` must answer with an ADMIN — note the path, it is not `/v1/me`. The panel reads
// `useAuth()`, and the add form, the switches and the per-source search are all admin-only, so a
// stub that misses this route renders a read-only list and every admin assertion fails for the
// wrong reason.
// ⚠ The catch-all answered EVERY non-`me` request with `{ sources: [], total: 0, results: [] }` —
// a UNION of three different endpoints' shapes, merged so that whichever one asked would find
// something plausible. That is the strongest possible form of the wrong-shape trap: it cannot fail,
// because it is pre-satisfied for every caller.
//
// ⚠ The `me` fixture also omitted `local`, which MeBody requires. `me()` carries it.
const stubSources = () => {
  const adds: unknown[] = [];
  const enables: { id: string; body: unknown }[] = [];
  server.use(
    getMeMockHandler(me({ name: "Admin" })),
    getAddFillerSourceMockHandler(async ({ request }) => {
      adds.push(await request.json());
      // ⚠ The add returns a RemoteSourceDTO — `{ id, label, uri, enabled }` — not a bare id.
      return { id: "src-new", label: "New source", uri: "/mnt/extra-ads", enabled: true };
    }),
    getSetFillerSourceEnabledMockHandler(async ({ request, params }) => {
      enables.push({ id: String(params.id), body: await request.json() });
      return { id: String(params.id), enabled: false };
    }),
    getListFillerSourcesMockHandler({ sources: [], total: 0 }),
  );
  return { adds, enables };
};

const source = (over: Partial<FillerSourceDTO> & Pick<FillerSourceDTO, "kind">): FillerSourceDTO => ({
  id: over.kind,
  target: "/data/filler",
  detail: "watched directly",
  count: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  switchable: true,
  removable: false,
  searchable: false,
  ...over,
});

const renderPanel = (sources: FillerSourceDTO[] = [source({ kind: "folder" })]) =>
  render(<SourcesPanel sources={sources} />, { wrapper: makeWrapper() });

describe("SourcesPanel", () => {
  it("lists the sources it is given", () => {
    stubSources();
    renderPanel([
      source({ kind: "folder", id: "folder", target: "/data/filler" }),
      source({ kind: "archive", id: "archive:classic", target: "classic_tv_commercials" }),
    ]);

    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.getByText("classic_tv_commercials")).toBeInTheDocument();
  });

  // ⚠ The kind is EXPLICIT on the wire, not sniffed from the URI. An archive identifier and a
  // YouTube playlist URL are validated by incompatible rules on the server — before the kind was
  // sent, the hardcoded archive validator rejected every playlist URL with a message about
  // archive.org collections.
  it("sends the chosen kind with the new source", async () => {
    const { adds } = stubSources();
    renderPanel();

    // The add form is admin-only, so it appears only once `/v1/auth/me` has resolved.
    const uri = await screen.findByRole("textbox");
    await userEvent.type(uri, "/mnt/extra-ads");
    await userEvent.click(screen.getByRole("button", { name: /add source/i }));

    await waitFor(() => {
      expect(adds).toEqual([{ kind: "library", uri: "/mnt/extra-ads" }]);
    });
  });

  // ⚠ THE promise the switch has to keep. Switching a source off withdraws it from future
  // scanning; it does NOT remove clips already in the catalog. An operator who reads "off" as
  // "my clips are gone" will switch it back on and re-download everything they already have.
  it("switching a source off sends only the enabled flag", async () => {
    const { enables } = stubSources();
    renderPanel([source({ kind: "folder", id: "folder", target: "/data/filler" })]);

    await userEvent.click(screen.getByRole("switch", { name: "Use /data/filler" }));

    await waitFor(() => {
      // The id is the PATH PARAM the resolver parsed, so this pins WHICH source was switched —
      // the old version asserted `patch?.url` contained a string the test had also written.
      expect(enables).toEqual([{ id: "folder", body: { enabled: false } }]);
    });
  });

  // ⚠ Only `archive` can be searched in place, and the panel reads the server's `searchable`
  // flag rather than testing the kind itself. A YouTube playlist can only be ENUMERATED by
  // yt-dlp, so a search box there would return nothing forever.
  // ⚠ Queried by the SOURCE's name, not the button's visible "Search it" (§10 V54 B6). Several
  // collections now appear together under one provider, and five buttons all reading "Search it"
  // are indistinguishable to anyone not looking at the row they sit in — so the accessible name
  // carries the target, and that is what a test must ask for.
  it("offers search only on a searchable source", () => {
    stubSources();
    const { unmount } = renderPanel([
      source({ kind: "archive", id: "a", target: "classic_tv", searchable: true }),
    ]);
    expect(screen.getByRole("button", { name: /^search classic_tv$/i })).toBeInTheDocument();
    unmount();

    renderPanel([source({ kind: "youtube", id: "y", target: "vintage_ads", searchable: false })]);
    expect(screen.queryByRole("button", { name: /^search vintage_ads$/i })).not.toBeInTheDocument();
  });

  // Registering records that a source EXISTS and is allowed. Downloading is the pull's approval
  // gate or a deliberate per-result queue — an operator who expects "add" to start fetching would
  // otherwise read the silence as a failure.
  it("says that adding a source downloads nothing", async () => {
    stubSources();
    renderPanel();
    expect(await screen.findByText(/downloads nothing/i)).toBeInTheDocument();
  });
});

// The per-source search expander (§10 V54 B6).
//
// ⚠ **`searchOpen` was ONE boolean for the whole panel**, and `renderSearch` is called once per
// source — so pressing "Search it" on one collection expanded the panel on EVERY searchable row,
// each showing the same query and the same results. It looked correct with a single collection
// registered, which is every fixture in this file before now. The provider roll-up is exactly what
// puts several collections on screen at once.
describe("SourcesPanel per-source search", () => {
  const twoCollections = [
    source({ kind: "archive", id: "archive:classic", target: "Classic TV Commercials", searchable: true }),
    source({ kind: "archive", id: "archive:psas", target: "Vintage PSAs", searchable: true }),
  ];

  it("opens the search on the row that was clicked, and only that row", async () => {
    stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: /^search Classic TV Commercials$/i }));

    // The clicked row is open…
    expect(
      screen.getByRole("button", { name: /close the search of Classic TV Commercials/i }),
    ).toBeInTheDocument();
    // …and the other row is still offering to open, rather than already showing this row's panel.
    expect(screen.getByRole("button", { name: /^search Vintage PSAs$/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /close the search of Vintage PSAs/i }),
    ).not.toBeInTheDocument();
  });

  // One panel at a time is what lets the query state stay single — and switching rows must not
  // leave the previous row's answers sitting under this row's name.
  it("moves the panel when a different source is searched", async () => {
    stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: /^search Classic TV Commercials$/i }));
    await userEvent.click(screen.getByRole("button", { name: /^search Vintage PSAs$/i }));

    expect(screen.getByRole("button", { name: /close the search of Vintage PSAs/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^search Classic TV Commercials$/i })).toBeInTheDocument();
  });

  it("closes the panel when its own button is pressed again", async () => {
    stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: /^search Classic TV Commercials$/i }));
    await userEvent.click(
      screen.getByRole("button", { name: /close the search of Classic TV Commercials/i }),
    );

    expect(screen.getByRole("button", { name: /^search Classic TV Commercials$/i })).toBeInTheDocument();
  });
});
