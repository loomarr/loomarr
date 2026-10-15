import { getMeMockHandler } from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { AdminOnly } from "./admin-only";

// A two-route tree: the gated page and the Home it offers, so "Go to Home" is asserted by where
// the router lands rather than by a spy on navigate.
const renderGated = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: Outlet });
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/private",
      component: () => (
        <AdminOnly>
          <p>Admin page body</p>
        </AdminOnly>
      ),
    }),
    createRoute({ getParentRoute: () => rootRoute, path: "/dashboard", component: () => <p>Home body</p> }),
  ]);
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: ["/private"] }) });
  render(
    <QueryClientProvider client={queryClient}>
      <LoomarrProvider theme="dark">
        <RouterProvider router={router} />
      </LoomarrProvider>
    </QueryClientProvider>,
  );
  return router;
};

describe("AdminOnly", () => {
  it("renders the page for an admin", async () => {
    server.use(getMeMockHandler(me()));
    renderGated();
    expect(await screen.findByText("Admin page body")).toBeInTheDocument();
    expect(screen.queryByText("Admins only")).not.toBeInTheDocument();
  });

  it("gives a member the no-access state instead of the page, with a way Home", async () => {
    server.use(getMeMockHandler(me({ role: "member" })));
    const router = renderGated();
    expect(await screen.findByText("Admins only")).toBeInTheDocument();
    expect(screen.queryByText("Admin page body")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Go to Home" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
    expect(await screen.findByText("Home body")).toBeInTheDocument();
  });
});
