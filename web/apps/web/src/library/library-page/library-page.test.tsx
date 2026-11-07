import {
  getLibraryStatusMockHandler,
  getListLibraryItemsMockHandler,
  getListLibraryNeedsMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
    server.use(getListLibraryNeedsMockHandler({ needs: [], problems: [] }));
  });

  it("lists the titles the server found and opens one", async () => {
    server.use(
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha, bravo], total: 2 }),
    );
    renderPage();

    // A title whose files aren't read yet says so in its name.
    expect(await screen.findByRole("button", { name: "Bravo, getting ready" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Alpha" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/$id", params: { id: "90000001" } });
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
