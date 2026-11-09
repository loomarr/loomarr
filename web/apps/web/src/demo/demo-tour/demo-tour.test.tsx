import { getSetupStateMockHandler } from "@loomarr/api/msw";
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
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { DemoTour, useDemoTour } from "./demo-tour";

const STORAGE_KEY = "loomarr.demo-tour";

const demoSite = (enabled: boolean) =>
  getSetupStateMockHandler({
    bootstrapped: true,
    devLogin: false,
    sso: false,
    demo: enabled
      ? { enabled: true, username: "guest", password: "loomarr-guest" }
      : { enabled: false, username: "", password: "" },
  });

// The tour sits in a layout above every page, as in _authed, so a visit to any page counts.
const Layout = () => {
  const tour = useDemoTour();
  return (
    <>
      <DemoTour tour={tour} channelId="ch-1" />
      <Outlet />
    </>
  );
};

const renderTour = (path = "/home") => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: Layout });
  const page = (routePath: string) =>
    createRoute({ getParentRoute: () => rootRoute, path: routePath, component: () => <p>{routePath}</p> });
  const routeTree = rootRoute.addChildren([
    page("/home"),
    page("/guide"),
    page("/watch/$id"),
    page("/channels/$id"),
  ]);
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) });
  render(
    <QueryClientProvider client={queryClient}>
      <LoomarrProvider theme="dark">
        <RouterProvider router={router} />
      </LoomarrProvider>
    </QueryClientProvider>,
  );
  return router;
};

const go = async (router: ReturnType<typeof renderTour>, to: string) => {
  await act(async () => {
    await router.navigate({ to });
  });
};

beforeEach(() => {
  window.localStorage.clear();
});

describe("DemoTour", () => {
  it("shows nothing unless the server says this is a demo site", async () => {
    server.use(demoSite(false));
    renderTour();
    expect(await screen.findByText("/home")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Demo tour" })).not.toBeInTheDocument();
  });

  it("walks through each step as its page is visited, then says the tour is done", async () => {
    server.use(demoSite(true));
    const router = renderTour();

    expect(await screen.findByText("Step 1 of 3")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Watch a channel live" })).toHaveAttribute("href", "/watch/ch-1");

    await go(router, "/watch/ch-1");
    expect(await screen.findByText("Step 2 of 3")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Browse the Guide" })).toHaveAttribute("href", "/guide");

    await go(router, "/guide");
    expect(await screen.findByText("Step 3 of 3")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Look inside a channel" })).toHaveAttribute(
      "href",
      "/channels/ch-1",
    );

    await go(router, "/channels/ch-1");
    expect(await screen.findByText(/That's the tour/)).toBeInTheDocument();
    await waitFor(() =>
      expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}").visited).toEqual([
        "watch",
        "guide",
        "channel",
      ]),
    );
  });

  // #2750: the finished banner stayed on every page until the visitor found the X. It says the tour
  // is done on the page that finished it, then goes.
  it("dismisses itself once the visitor moves on from the finished tour", async () => {
    server.use(demoSite(true));
    const router = renderTour("/watch/ch-1");
    expect(await screen.findByText("Step 2 of 3")).toBeInTheDocument();
    await go(router, "/guide");
    expect(await screen.findByText("Step 3 of 3")).toBeInTheDocument();
    await go(router, "/channels/ch-1");
    expect(await screen.findByText(/That's the tour/)).toBeInTheDocument();

    await go(router, "/home");
    expect(await screen.findByText("/home")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Demo tour" })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}").dismissed).toBe(true),
    );
  });

  it("stays hidden once dismissed, across a reload", async () => {
    server.use(demoSite(true));
    renderTour();
    await userEvent.click(await screen.findByRole("button", { name: "Hide the demo tour" }));
    expect(screen.queryByRole("region", { name: "Demo tour" })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}").dismissed).toBe(true),
    );
  });

  it("starts over when saved progress is unreadable", async () => {
    window.localStorage.setItem(STORAGE_KEY, "{not json");
    server.use(demoSite(true));
    renderTour();
    expect(await screen.findByText("Step 1 of 3")).toBeInTheDocument();
  });

  it("picks up where the visitor left off", async () => {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ visited: ["watch", "nonsense"], dismissed: false }),
    );
    server.use(demoSite(true));
    renderTour();
    expect(await screen.findByText("Step 2 of 3")).toBeInTheDocument();
  });
});
