import {
  getLibraryStatusMockHandler,
  getListCollectionsMockHandler,
  getListLibraryItemsMockHandler,
  getListLibraryNeedsMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryPage } from "./library-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mocks.navigate }));

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
    server.use(getListLibraryNeedsMockHandler({ needs: [], orders: [], problems: [] }));
  });

  it("lists the titles the server found and opens one", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha, bravo], total: 2 }),
    );
    renderPage();

    // A title whose files aren't read yet says so in its name, which is what its tile shows.
    // jsdom lays nothing out, so the tile's lines join without spaces in the computed name.
    expect(await screen.findByRole("button", { name: /^Getting ready\s*Bravo/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /^Alpha/ }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/$id", params: { id: "90000001" } });
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
    expect(await screen.findByRole("button", { name: /^Alpha/ })).toBeInTheDocument();
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
    expect(await screen.findByRole("button", { name: /^Getting ready\s*Bravo/ })).toBeInTheDocument();
  });

  it("asks for the URL's filter, then the typed search, and keeps the search in the URL", async () => {
    const asked: URLSearchParams[] = [];
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/items", ({ request }) => {
        asked.push(new URL(request.url).searchParams);
        return HttpResponse.json({ items: [alpha], total: 1 });
      }),
    );
    renderPage({ filter: "movie", search: "" });

    await waitFor(() => expect(asked.at(-1)?.get("kind")).toBe("movie"));
    await userEvent.type(await screen.findByLabelText("Search your library"), "alp");

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
    );
    renderPage({ filter: "collections", search: "", view: "collections" });

    await userEvent.click(await screen.findByRole("button", { name: "Edit Rainy Sundays" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/collections/$id", params: { id: "c1" } });
    expect(screen.queryByLabelText("Search your library")).not.toBeInTheDocument();
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
});
