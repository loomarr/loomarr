import {
  getHouseholdMockHandler,
  getLibraryStatusMockHandler,
  getListLibraryGenresMockHandler,
  getListLibraryItemsMockHandler,
  getListLibraryNeedsMockHandler,
  getMeMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { libraryStatus } from "@/test/fixtures/library";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryPage } from "./library-page";

const alpha = { id: "90000001", kind: "movie" as const, name: "Alpha", year: 2001, ready: true };

// A title's card is the router's own link, so it is driven by a real router here.
describe("LibraryPage's cards", () => {
  it("are links to the title's page, which a plain click follows in the app", async () => {
    server.use(
      getMeMockHandler(me()),
      getHouseholdMockHandler({ timeZone: "" }),
      getListLibraryGenresMockHandler({ genres: [] }),
      getListLibraryNeedsMockHandler({ needs: [], orders: [], problems: [] }),
      getLibraryStatusMockHandler(libraryStatus()),
      getListLibraryItemsMockHandler({ items: [alpha], total: 1 }),
    );
    const root = createRootRoute({ component: Outlet });
    const wall = createRoute({
      getParentRoute: () => root,
      path: "/library",
      component: () => <LibraryPage filter="all" search="" />,
    });
    const title = createRoute({
      getParentRoute: () => root,
      path: "/library/$id",
      component: () => <p>the title's page</p>,
    });
    const router = createRouter({
      routeTree: root.addChildren([wall, title]),
      history: createMemoryHistory({ initialEntries: ["/library"] }),
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router as never} />
      </QueryClientProvider>,
    );

    const card = await screen.findByRole("link", { name: /^Alpha/ });
    expect(card).toHaveAttribute("href", "/library/90000001");
    await userEvent.click(card);
    expect(await screen.findByText("the title's page")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/library/90000001");
  });
});
