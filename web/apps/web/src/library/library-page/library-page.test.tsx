import {
  getGetLibraryPersonMockHandler,
  getHouseholdMockHandler,
  getLibraryStatusMockHandler,
  getListCollectionsMockHandler,
  getListLibraryFacetsMockHandler,
  getListLibraryGenresMockHandler,
  getListLibraryItemsMockHandler,
  getListLibraryNeedsMockHandler,
  getMeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type ReactNode, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { libraryStatus } from "@/test/fixtures/library";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryPage } from "./library-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn(), width: 1440 }));
// The cards' links are the real router's in library-page-links.test.tsx; here a card only has to draw.
vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mocks.navigate,
  Link: ({ params, to, ...rest }: { params: { id: string }; to: string; children?: ReactNode }) => (
    <a {...rest} href={to.replace("$id", params.id)} />
  ),
}));
// A desktop window, where the wall draws its letter rail.
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: mocks.width }) };
});

const alpha = {
  id: "90000001",
  kind: "movie" as const,
  name: "Alpha",
  year: 2001,
  files: 1,
  ready: true,
  unplayable: 0,
  extraCopies: 0,
};
const bravo = {
  id: "90000002",
  kind: "series" as const,
  name: "Bravo",
  files: 4,
  ready: false,
  unplayable: 0,
  extraCopies: 0,
};

const facets = {
  studios: [{ value: "Example Studio", name: "Example Studio", titles: 4 }],
  audio: [{ value: "eng", name: "English", titles: 3 }],
  subtitles: [{ value: "fre", name: "French", titles: 2 }],
  hdr: [{ value: "HDR10", name: "HDR10", titles: 1 }],
  uhd: 2,
  hdrAny: 1,
};

const renderPage = (props: Parameters<typeof LibraryPage>[0] = { filter: "all", search: "" }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <LibraryPage {...props} />
    </QueryClientProvider>,
  );
};

// The wiring: the page lists what the server found under the library's status, asks for the
// URL's filter and the typed search, keeps the search in the URL, and opens a title's page.
describe("LibraryPage", () => {
  beforeEach(() => {
    mocks.navigate.mockReset();
    // The page tells times in the household's zone, which the household read carries.
    server.use(
      getMeMockHandler(me()),
      getListLibraryNeedsMockHandler({ needs: [], orders: [], problems: [] }),
      getHouseholdMockHandler({ timeZone: "" }),
      getListLibraryGenresMockHandler({ genres: [] }),
      getListLibraryFacetsMockHandler(facets),
    );
  });

  // The tabs' counts ask for one title of each kind; the grid's pages ask for many.
  const isCount = (url: URL) => url.searchParams.get("limit") === "1";

  it("marks a title its list says is unidentified, until the list stops saying so, asking nothing of Needs you", async () => {
    let unidentified = true;
    let needsRequests = 0;
    server.use(
      getLibraryStatusMockHandler({ ...libraryStatus(), needsYou: 1 }),
      http.get("*/v1/library/items", () =>
        HttpResponse.json({ items: [{ ...alpha, unidentified }], total: 1 }),
      ),
      http.get("*/v1/library/needs", () => {
        needsRequests += 1;
        return HttpResponse.json({ needs: [], orders: [], problems: [] });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <LibraryPage filter="all" search="" />
      </QueryClientProvider>,
    );

    const card = await screen.findByRole("link", { name: /Alpha/ });
    expect(card).toHaveTextContent("2001 · Not identified yet");
    expect(card).not.toHaveTextContent("Movie");
    expect(within(card).getByRole("img", { name: "Needs a look" })).toBeInTheDocument();
    // The status line has its count, so a read the status could start would be under way.
    await screen.findByRole("button", { name: /Review/ });

    // Answering a title refetches the titles.
    unidentified = false;
    await act(() => client.invalidateQueries({ queryKey: ["/v1/library/items"] }));
    await waitFor(() => expect(card).toHaveTextContent("2001 · Movie"));
    expect(within(card).queryByRole("img", { name: "Needs a look" })).not.toBeInTheDocument();
    expect(needsRequests).toBe(0);
  });

  it("lists the titles the server found, each linking to its page", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha, bravo], total: 2 }),
    );
    renderPage();

    // A title whose files aren't read yet says so under its name, which is what its tile shows.
    // jsdom lays nothing out, so the tile's lines join without spaces in the computed name.
    expect(await screen.findByRole("link", { name: /^Bravo\s*Getting ready$/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Alpha/ })).toHaveAttribute("href", "/library/90000001");
  });

  it("fills the grid and moves the status from the event stream, polling nothing", async () => {
    let listed = 0;
    let items: (typeof alpha | typeof bravo)[] = [alpha];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", () => {
        listed++;
        return HttpResponse.json({ items, total: items.length });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <LoomarrEventsProvider>
          <LibraryPage filter="all" search="" />
        </LoomarrEventsProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("link", { name: /^Alpha/ })).toBeInTheDocument();
    const stream = FakeEventSource.instances.at(-1);
    if (!stream) throw new Error("no event stream");

    // A scan frame is written into the status line; nothing is refetched for it.
    const before = listed;
    act(() =>
      stream.emit("library_scan", {
        step: "walking",
        stepNumber: 1,
        done: 112,
        total: 533,
        etaSeconds: 540,
        paused: false,
        quiet: false,
      }),
    );
    expect(
      await screen.findByText("Adding your media · Step 1 of 3 · 21% · about 9 min left"),
    ).toBeInTheDocument();
    expect(listed).toBe(before);

    // A titles frame refetches the grid, and the new title shows up.
    items = [alpha, bravo];
    act(() => stream.emit("library_titles", {}));
    expect(await screen.findByRole("link", { name: /^Bravo\s*Getting ready$/ })).toBeInTheDocument();
  });

  it("loads the next page when the grid reaches its last rows, and stops at the total", async () => {
    const offsets: string[] = [];
    const page = (from: number, n: number) =>
      Array.from({ length: n }, (_, i) => ({
        ...alpha,
        id: String(90200000 + from + i),
        name: `T${from + i}`,
      }));
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (isCount(url)) return HttpResponse.json({ items: [], total: 18 });
        const offset = Number(url.searchParams.get("offset") ?? 0);
        offsets.push(String(offset));
        return HttpResponse.json({ items: page(offset, offset === 0 ? 12 : 6), total: 18 });
      }),
    );
    renderPage();

    // A first page short enough to show its last rows asks for the next one, and no more.
    expect(await screen.findByRole("link", { name: /^T17/ })).toBeInTheDocument();
    expect(offsets).toEqual(["0", "12"]);
  });

  it("asks for the URL's filter, then the typed search, and keeps the search in the URL", async () => {
    const asked: URLSearchParams[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (!isCount(url)) asked.push(url.searchParams);
        return HttpResponse.json({ items: [alpha], total: 1 });
      }),
    );
    renderPage({ filter: "movie", search: "" });

    await waitFor(() => expect(asked.at(-1)?.get("kind")).toBe("movie"));
    await userEvent.type(await screen.findByLabelText("Search titles"), "alp");

    await waitFor(() => expect(asked.at(-1)?.get("search")).toBe("alp"));
    expect(asked.at(-1)?.get("kind")).toBe("movie");
    // One URL update for the settled search, replacing the entry rather than adding one per key.
    expect(mocks.navigate).toHaveBeenCalledWith({
      to: "/library",
      search: { kind: "movie", q: "alp" },
      replace: true,
    });
  });

  it("shows the collections on the Collections tab, and opens one to edit", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListCollectionsMockHandler({
        collections: [{ id: "c1", name: "Rainy Sundays", description: "", titles: [alpha], usedBy: [] }],
        sets: [],
      }),
      // The tabs keep their counts on Collections.
      getListLibraryItemsMockHandler({ items: [alpha], total: 1 }),
    );
    renderPage({ filter: "collections", search: "", view: "collections" });

    await userEvent.click(await screen.findByRole("button", { name: "Edit Rainy Sundays" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/collections/$id", params: { id: "c1" } });
    expect(screen.queryByLabelText("Search titles")).not.toBeInTheDocument();
  });

  it("goes to the Collections tab's own address", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha], total: 1 }),
    );
    renderPage();

    await userEvent.click(await screen.findByRole("tab", { name: "Collections" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/collections" });
  });

  it("shows why the library couldn't be read", async () => {
    server.use(
      http.get("*/v1/library/status", () =>
        HttpResponse.json(
          { title: "Boom", status: 500 },
          { status: 500, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
      getListLibraryItemsMockHandler({ items: [], total: 0 }),
    );
    renderPage();

    expect(await screen.findByRole("button", { name: /try again/i })).toBeInTheDocument();
  });

  it("says how many need you from the status, and Review opens Needs you's own page", async () => {
    server.use(
      getLibraryStatusMockHandler({ ...libraryStatus(), needsYou: 6 }),
      getListLibraryItemsMockHandler({ items: [alpha], total: 1 }),
    );
    renderPage();

    expect(await screen.findByText("6 need you")).toBeInTheDocument();
    await userEvent.click(screen.getByText("Review"));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/needs" });
  });

  it("counts each tab across the whole library", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (!isCount(url)) return HttpResponse.json({ items: [alpha], total: 1 });
        const kind = url.searchParams.get("kind");
        const total = kind === "movie" ? 2180 : kind === "series" ? 1232 : 3412;
        return HttpResponse.json({ items: [alpha], total });
      }),
    );
    // The tabs carry the counts on a wide window and a phone says the chosen tab's beside the
    // order (both pinned in @loomarr/ui); either way the tab's count is the library's.
    const all = renderPage();
    expect(await screen.findByText(/3,412/)).toBeInTheDocument();
    all.unmount();

    renderPage({ filter: "movie", search: "" });
    expect(await screen.findByText(/2,180/)).toBeInTheDocument();
  });

  it("asks for the titles added last when the order says so, and keeps it in the URL", async () => {
    const asked: URLSearchParams[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (!isCount(url)) asked.push(url.searchParams);
        return HttpResponse.json({ items: [alpha], total: 1 });
      }),
    );
    renderPage({ filter: "all", search: "" });

    await waitFor(() => expect(asked.at(-1)?.get("sort")).toBeNull());
    await userEvent.selectOptions(await screen.findByLabelText("Order"), "recent");
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library", search: { sort: "recent" } });

    renderPage({ filter: "all", search: "", sort: "recent" });
    await waitFor(() => expect(asked.at(-1)?.get("sort")).toBe("recent"));
  });

  // #4476: a member browses the wall read-only. Every request it makes is one a member may make;
  // the status, Needs you and the folders are an admin's and answer a member 403.
  it("gives a member the wall and its collections without asking for anything admin-only", async () => {
    const paths: string[] = [];
    server.events.on("request:start", ({ request }) => {
      paths.push(new URL(request.url).pathname);
    });
    server.use(
      getMeMockHandler(me({ role: "member" })),
      getListLibraryItemsMockHandler({ items: [{ ...alpha, files: undefined }], total: 1 }),
      getListCollectionsMockHandler({ collections: [], sets: [] }),
    );
    try {
      const wall = renderPage();
      expect(await screen.findByRole("link", { name: /^Alpha/ })).toHaveAttribute(
        "href",
        "/library/90000001",
      );
      expect(screen.queryByText(/need you/)).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Library settings" })).not.toBeInTheDocument();
      wall.unmount();

      renderPage({ filter: "collections", search: "", view: "collections" });
      expect(await screen.findByText("No collections yet")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "New collection" })).not.toBeInTheDocument();
    } finally {
      server.events.removeAllListeners();
    }
    const library = paths.filter((p) => p.startsWith("/v1/library") || p.startsWith("/v1/collections"));
    expect(library.length).toBeGreaterThan(0);
    for (const path of library) {
      expect(["/v1/library/items", "/v1/library/genres", "/v1/library/facets", "/v1/collections"]).toContain(
        path,
      );
    }
  });

  it("asks for the URL's genre, keeps it in the URL, and keeps it across a tab change", async () => {
    const asked: URLSearchParams[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryGenresMockHandler({
        genres: [
          { name: "Drama", titles: 3 },
          { name: "Comedy", titles: 2 },
        ],
      }),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (!isCount(url)) asked.push(url.searchParams);
        return HttpResponse.json({ items: [alpha], total: 1 });
      }),
    );
    renderPage({ filter: "all", search: "", genre: "Drama" });

    await waitFor(() => expect(asked.at(-1)?.get("genre")).toBe("Drama"));
    await screen.findByRole("option", { name: "Comedy · 2" });
    await userEvent.selectOptions(screen.getByLabelText("Genre"), "Comedy");
    expect(mocks.navigate).toHaveBeenLastCalledWith({ to: "/library", search: { genre: "Comedy" } });
    await userEvent.selectOptions(screen.getByLabelText("Genre"), "");
    expect(mocks.navigate).toHaveBeenLastCalledWith({ to: "/library", search: {} });
    await userEvent.click(screen.getByRole("tab", { name: /^Movies/ }));
    expect(mocks.navigate).toHaveBeenLastCalledWith({
      to: "/library",
      search: { kind: "movie", genre: "Drama" },
    });
    await userEvent.type(screen.getByLabelText("Search titles"), "alp");
    await waitFor(() =>
      expect(mocks.navigate).toHaveBeenLastCalledWith({
        to: "/library",
        search: { q: "alp", genre: "Drama" },
        replace: true,
      }),
    );
  });

  it("lists the new tab's genres when the tab changes", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha], total: 1 }),
      http.get("*/v1/library/genres", ({ request }) => {
        const kind = new URL(request.url).searchParams.get("kind");
        return HttpResponse.json({
          genres: [{ name: kind === "series" ? "Reality" : "Western", titles: 1 }],
        });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const page = (filter: "movie" | "series") => (
      <QueryClientProvider client={client}>
        <LibraryPage filter={filter} search="" />
      </QueryClientProvider>
    );
    const view = render(page("movie"));
    expect(await screen.findByRole("option", { name: "Western · 1" })).toBeInTheDocument();

    view.rerender(page("series"));
    expect(await screen.findByRole("option", { name: "Reality · 1" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Western · 1" })).not.toBeInTheDocument();
  });

  // The wall is one list from its first title, so a jump loads on until the letter's title and a
  // page below it are in, in pages as large as the server gives.
  it("loads as far as a rail letter's title when it is pressed", async () => {
    const asked: string[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (isCount(url)) return HttpResponse.json({ items: [], total: 1000 });
        const offset = Number(url.searchParams.get("offset") ?? 0);
        const limit = Number(url.searchParams.get("limit"));
        asked.push(`${offset}:${limit}`);
        // The first page is long enough that the grid asks for no second one by itself.
        const length = offset === 0 ? 200 : limit;
        const items = Array.from({ length }, (_, i) => ({
          ...alpha,
          id: String(90300000 + offset + i),
          name: `T${offset + i}`,
        }));
        return HttpResponse.json({
          items,
          total: 1000,
          letters: [
            { letter: "A", offset: 0 },
            { letter: "T", offset: 400 },
          ],
        });
      }),
    );
    renderPage();

    const rail = await screen.findByRole("button", { name: "Jump to titles starting with T" });
    expect(asked).toEqual(["0:60"]);
    await userEvent.click(rail);

    await waitFor(() => expect(asked).toEqual(["0:60", "200:200", "400:60"]));
  });

  // A wall of `total` titles whose letter T starts at 400, served in the pages asked for. The
  // first page is long enough that the grid asks for no second one by itself. `fail` is how many
  // requests past the first page answer 500, and `hold` keeps those requests waiting.
  const longWall = ({
    total = 1000,
    fail = 0,
    hold,
  }: {
    total?: number;
    fail?: number;
    hold?: Promise<void>;
  } = {}) => {
    const wall = { asked: [] as string[], failures: fail };
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", async ({ request }) => {
        const url = new URL(request.url);
        if (isCount(url)) return HttpResponse.json({ items: [], total });
        const offset = Number(url.searchParams.get("offset") ?? 0);
        const limit = Number(url.searchParams.get("limit"));
        wall.asked.push(`${offset}:${limit}`);
        if (offset > 0) await hold;
        if (offset > 0 && wall.failures > 0) {
          wall.failures--;
          return HttpResponse.json({ title: "Boom", status: 500 }, { status: 500 });
        }
        const length = Math.min(offset === 0 ? Math.max(200, limit) : limit, total - offset);
        const items = Array.from({ length }, (_, i) => ({
          ...alpha,
          id: String(90300000 + offset + i),
          name: `T${offset + i}`,
        }));
        return HttpResponse.json({
          items,
          total,
          letters: [
            { letter: "B", offset: 10 },
            { letter: "H", offset: 150 },
            { letter: "T", offset: 400 },
          ],
        });
      }),
    );
    return wall;
  };
  const letter = (name: string) =>
    screen.findByRole("button", { name: `Jump to titles starting with ${name}` });

  it("stops at the end of the list when the letter is near it", async () => {
    const wall = longWall({ total: 430 });
    renderPage();

    await userEvent.click(await letter("T"));
    await waitFor(() => expect(wall.asked).toEqual(["0:60", "200:200", "400:60"]));
    await waitFor(async () => expect(await letter("T")).not.toHaveAttribute("aria-busy"));
    expect(wall.asked).toHaveLength(3);
  });

  it("shows a failed page, and asks once more on the next press", async () => {
    const wall = longWall({ fail: 1 });
    renderPage();

    await userEvent.click(await letter("T"));
    expect(await screen.findByRole("button", { name: /try again/i })).toBeInTheDocument();
    expect(await letter("T")).not.toHaveAttribute("aria-busy");
    expect(wall.asked).toEqual(["0:60", "200:200"]);

    await userEvent.click(await letter("T"));
    await waitFor(() => expect(wall.asked).toEqual(["0:60", "200:200", "200:200", "400:60"]));
  });

  it("scrolls to a loaded title even when the page below it fails", async () => {
    const scrollTo = vi.fn();
    Object.assign(window, { scrollTo });
    Object.assign(Element.prototype, { scrollTo });
    // The grid scrolls whatever holds it, and jsdom has no such thing until one says it scrolls.
    document.body.style.overflowY = "auto";
    const wall = longWall({ fail: 1 });
    renderPage();

    // H's title, the 151st, is on the first page of 200; fewer than a page sit under it, so the
    // next page is asked for, and that one fails.
    await letter("H");
    scrollTo.mockClear();
    await userEvent.click(await letter("H"));
    await screen.findByRole("button", { name: /try again/i });
    // The wall is scrolled, though the page below never came. jsdom lays nothing out, so the
    // virtualizer clamps the position to 0; which row it asks for is the shelf's test.
    await waitFor(() => expect(scrollTo).toHaveBeenCalled());
    expect(wall.asked).toEqual(["0:60", "200:60"]);
    document.body.style.overflowY = "";
  });

  it("shows the pressed letter busy, and takes the latest press made during a fetch", async () => {
    let release = () => {};
    const wall = longWall({ hold: new Promise<void>((resolve) => (release = resolve)) });
    renderPage();

    await userEvent.click(await letter("T"));
    expect(await letter("T")).toHaveAttribute("aria-busy", "true");
    await userEvent.click(await letter("B"));
    expect(await letter("T")).not.toHaveAttribute("aria-busy");
    release();
    await waitFor(() => expect(wall.asked).toEqual(["0:60", "200:200"]));
  });

  it("drops a loading jump when the reader scrolls", async () => {
    let release = () => {};
    const wall = longWall({ hold: new Promise<void>((resolve) => (release = resolve)) });
    renderPage();

    await userEvent.click(await letter("T"));
    expect(await letter("T")).toHaveAttribute("aria-busy", "true");
    fireEvent.wheel(window);
    expect(await letter("T")).not.toHaveAttribute("aria-busy");
    release();
    await waitFor(async () => expect(await letter("T")).toBeInTheDocument());
    expect(wall.asked).toEqual(["0:60", "200:200"]);
  });

  it("forgets a jump when the tab changes, and does not revive it on the way back", async () => {
    let release = () => {};
    const wall = longWall({ hold: new Promise<void>((resolve) => (release = resolve)) });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const page = (filter: "all" | "movie") => (
      <QueryClientProvider client={client}>
        <LibraryPage filter={filter} search="" />
      </QueryClientProvider>
    );
    const view = render(page("all"));

    await userEvent.click(await letter("T"));
    view.rerender(page("movie"));
    await letter("T");
    view.rerender(page("all"));
    expect(await letter("T")).not.toHaveAttribute("aria-busy");
    release();
    await waitFor(async () => expect(await letter("T")).toBeInTheDocument());
    expect(wall.asked.filter((a) => a.startsWith("400:"))).toEqual([]);
  });

  it("keeps as many titles when the list is read again after a deep jump", async () => {
    const wall = longWall();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const page = (
      <QueryClientProvider client={client}>
        <LibraryPage filter="all" search="" />
      </QueryClientProvider>
    );
    const first = render(page);
    await userEvent.click(await letter("T"));
    await waitFor(() => expect(wall.asked).toHaveLength(3));
    await waitFor(async () => expect(await letter("T")).not.toHaveAttribute("aria-busy"));
    // The page is left and come back to, so the jump's reach is gone, and the list is read again.
    first.unmount();
    wall.asked.length = 0;
    render(page);
    await act(() => client.invalidateQueries());

    await waitFor(() => expect(wall.asked.slice(-3)).toEqual(["0:200", "200:200", "400:60"]));
  });

  it("draws no rail by Recently added, even if letters come back", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha], total: 1, letters: [{ letter: "A", offset: 0 }] }),
    );
    renderPage({ filter: "all", search: "", sort: "recent" });

    expect(await screen.findByRole("link", { name: /^Alpha/ })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Jump to a letter" })).not.toBeInTheDocument();
  });

  // Every title list asked for, past the tabs' one-title counts.
  const listsAsked = (body: { items: (typeof alpha)[]; total: number }) => {
    const asked: URLSearchParams[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        if (isCount(url)) return HttpResponse.json({ items: [alpha], total: 9 });
        asked.push(url.searchParams);
        return HttpResponse.json(body);
      }),
    );
    return asked;
  };
  const openFilters = () => userEvent.click(screen.getByRole("button", { name: /^More filters/ }));
  const lastSearch = () => mocks.navigate.mock.calls.at(-1)?.[0].search;

  it.each([
    ["Studio", "Example Studio", { studio: "Example Studio" }],
    ["Audio language", "eng", { audio: "eng" }],
    ["Subtitle language", "fre", { subtitles: "fre" }],
  ])("puts the %s filter in the URL", async (label, value, expected) => {
    listsAsked({ items: [alpha], total: 1 });
    renderPage();
    await screen.findByRole("link", { name: /^Alpha/ });
    await openFilters();
    await screen.findByRole("option", { name: "Example Studio · 4" });

    await userEvent.selectOptions(screen.getByRole("combobox", { name: label }), value);
    expect(lastSearch()).toEqual(expected);
  });

  it.each([
    ["4K", { uhd: true }],
    ["HDR", { hdr: "any" }],
  ])("puts the %s toggle in the URL", async (label, expected) => {
    listsAsked({ items: [alpha], total: 1 });
    renderPage();
    await screen.findByRole("link", { name: /^Alpha/ });
    await openFilters();

    await userEvent.click(screen.getByRole("switch", { name: new RegExp(`^${label}`) }));
    expect(lastSearch()).toEqual(expected);
  });

  it("asks for every filter in the URL, lists what is on, and counts the titles they leave", async () => {
    const asked = listsAsked({ items: [alpha], total: 3 });
    renderPage({
      filter: "all",
      search: "",
      genre: "Drama",
      filters: {
        studio: "Example Studio",
        audio: "eng",
        subtitles: "fre",
        uhd: true,
        hdr: "any",
      },
    });

    await waitFor(() => expect(asked.length).toBeGreaterThan(0));
    const params = Object.fromEntries(asked.at(-1) ?? []);
    expect(params).toMatchObject({
      genre: "Drama",
      studio: "Example Studio",
      audio: "eng",
      subtitles: "fre",
      uhd: "true",
      hdr: "any",
    });
    expect(await screen.findByRole("button", { name: "Remove filter: Audio: English" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "More filters (5)" })).toBeInTheDocument();
    expect(await screen.findByText("3 titles")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Remove filter: 4K" }));
    expect(lastSearch()).toEqual({
      genre: "Drama",
      studio: "Example Studio",
      audio: "eng",
      subtitles: "fre",
      hdr: "any",
    });
    await userEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(lastSearch()).toEqual({});
  });

  it("searches people, sets the chosen one in the URL, and a member may", async () => {
    const searched: string[] = [];
    listsAsked({ items: [alpha], total: 1 });
    server.use(
      getMeMockHandler(me({ role: "member" })),
      http.get("*/v1/library/people", ({ request }) => {
        searched.push(new URL(request.url).searchParams.get("q") ?? "");
        return HttpResponse.json({ people: [{ id: "p1", name: "Ada Example", titles: 2 }] });
      }),
    );
    renderPage();
    await screen.findByRole("link", { name: /^Alpha/ });
    await openFilters();

    await userEvent.type(screen.getByRole("combobox", { name: "Person" }), "ada");
    await userEvent.click(await screen.findByRole("option", { name: /Ada Example/ }));
    expect(lastSearch()).toEqual({ person: "p1" });
    // Settled before asking: one request for the word, not one for each letter.
    expect(searched).toEqual(["ada"]);
  });

  it("asks for no people until two characters are typed", async () => {
    const searched: string[] = [];
    listsAsked({ items: [alpha], total: 1 });
    server.use(
      http.get("*/v1/library/people", ({ request }) => {
        searched.push(new URL(request.url).searchParams.get("q") ?? "");
        return HttpResponse.json({ people: [] });
      }),
    );
    renderPage();
    await screen.findByRole("link", { name: /^Alpha/ });
    await openFilters();
    const field = screen.getByRole("combobox", { name: "Person" });
    // Past the settle time, so a request would have gone.
    const settle = () => act(() => new Promise((done) => setTimeout(done, 400)));

    await userEvent.type(field, " a ");
    await settle();
    expect(searched).toEqual([]);
    await userEvent.type(field, "d");
    await settle();
    await waitFor(() => expect(searched).toEqual(["a d"]));
  });

  // The page with the URL's filters following what it navigates to, as the route would.
  const Routed = ({ start }: { start: NonNullable<Parameters<typeof LibraryPage>[0]["filters"]> }) => {
    const [filters, setFilters] = useState(start);
    mocks.navigate.mockImplementation(({ search }) => setFilters(search));
    return <LibraryPage filter="all" filters={filters} search="" />;
  };
  const renderRouted = (start: Parameters<typeof Routed>[0]["start"]) =>
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Routed start={start} />
      </QueryClientProvider>,
    );

  it("moves focus to a neighbouring chip, then More filters, as chips are removed", async () => {
    listsAsked({ items: [alpha], total: 3 });
    renderRouted({ audio: "eng", uhd: true, hdr: "any" });
    const chip = (name: string) => screen.getByRole("button", { name: `Remove filter: ${name}` });
    await screen.findByRole("button", { name: "Remove filter: Audio: English" });

    await userEvent.click(chip("4K"));
    expect(document.activeElement).toBe(chip("HDR"));
    await userEvent.click(chip("HDR"));
    expect(document.activeElement).toBe(chip("Audio: English"));
    await userEvent.click(chip("Audio: English"));
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "More filters" }));
  });

  it.each([
    ["the line of what is on", [alpha], ""],
    ["the empty panel", [], ""],
  ])("moves focus to More filters on Clear filters in %s", async (_where, items) => {
    listsAsked({ items, total: items.length });
    renderRouted({ uhd: true });

    await userEvent.click(await screen.findByRole("button", { name: "Clear filters" }));
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "More filters" }));
  });

  it("keeps the count's live region from the start, so a new count is announced", async () => {
    let answer = () => {};
    const held = new Promise<void>((done) => {
      answer = done;
    });
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", async ({ request }) => {
        if (!isCount(new URL(request.url))) await held;
        return HttpResponse.json({ items: [alpha], total: 3 });
      }),
    );
    renderRouted({ uhd: true });
    // The page is up and the list is still on its way.
    const region = await screen.findByRole("status", { name: "Filtered titles" });
    expect(region).toBeEmptyDOMElement();

    answer();
    // The same element, filled.
    await waitFor(() => expect(region).toHaveTextContent("3 titles"));
  });

  it("names a person arriving in the URL on a chip that removes them", async () => {
    const asked = listsAsked({ items: [alpha], total: 1 });
    server.use(getGetLibraryPersonMockHandler({ id: "p1", name: "Ada Example", titles: 2 }));
    renderPage({ filter: "all", search: "", filters: { person: "p1" } });

    const chip = await screen.findByRole("button", { name: "Remove filter: Titles with Ada Example" });
    expect(chip).toHaveTextContent("Titles with Ada Example");
    expect(asked.at(-1)?.get("person")).toBe("p1");
    await userEvent.click(chip);
    expect(lastSearch()).toEqual({});
  });

  it("offers Clear filters when the filters leave no titles", async () => {
    listsAsked({ items: [], total: 0 });
    renderPage({ filter: "all", search: "", filters: { uhd: true } });

    expect(await screen.findByText("No titles match these filters")).toBeInTheDocument();
    const clear = screen.getByRole("button", { name: "Clear filters" });
    await userEvent.click(clear);
    expect(lastSearch()).toEqual({});
  });

  it("says a search that finds nothing is inside the filters, and clears them", async () => {
    listsAsked({ items: [], total: 0 });
    renderPage({ filter: "all", search: "zebra", filters: { uhd: true } });

    expect(await screen.findByText(/only in titles that match your filters/)).toBeInTheDocument();
    expect(screen.queryByText(/across all/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(lastSearch()).toEqual({ q: "zebra" });
  });
});
