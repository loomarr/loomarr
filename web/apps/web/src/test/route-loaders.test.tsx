import type { JobView } from "@loomarr/api";
import { getJobsListMockHandler, getListLibraryRootsMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RouteError } from "@/components/loomarr/feedback/route-error";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";

// A page whose loader ensures the data it is made of (#3399 step 9.2): it opens on that data, and a
// first read that fails is the route's error view, with a retry, rather than a page of its own that
// paints around a loading or error panel.

const clients: QueryClient[] = [];
afterEach(async () => {
  await Promise.all(clients.splice(0).map((c) => vi.waitFor(() => expect(c.isFetching()).toBe(0))));
});

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(queryClient);
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
    defaultErrorComponent: RouteError,
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

const job: JobView = {
  name: "reconcile",
  group: "acquisitions",
  title: "Reconcile acquisitions",
  description: "Advances approved requests through download, retry, and completion states.",
  paused: false,
  schedule: "0 */5 * * * *",
  scheduleKey: "job.reconcile.schedule",
  lastResult: "ok",
  lastRun: "2026-07-23T12:00:00Z",
  nextRun: "2026-07-23T12:05:00Z",
  running: false,
};

const folder = {
  path: "/media/movies",
  inUse: true,
  readable: true,
  empty: false,
  addedByHand: false,
  checkedAt: "2026-07-25T19:55:00Z",
  foundIn: [{ authority: "radarr" as const, path: "/media/movies" }],
};

const failing = (endpoint: string) => {
  const state = { failing: true };
  const handler = (answer: () => Response) =>
    http.get(endpoint, () =>
      state.failing ? HttpResponse.json({ title: "Server is restarting" }, { status: 503 }) : answer(),
    );
  return { state, handler };
};

describe("Settings → Tasks", () => {
  it("opens on its job list", async () => {
    server.use(getMeMockHandler(me()), getJobsListMockHandler({ jobs: [job] }), ...appHandlers());
    renderAt("/settings/system/tasks");

    expect(await screen.findByText("Acquisitions")).toBeInTheDocument();
    expect(screen.queryByText("No scheduled tasks.")).not.toBeInTheDocument();
  });

  it("shows a failed first read as the route's error view, and recovers on Try again", async () => {
    const jobs = failing("*/v1/jobs");
    server.use(
      getMeMockHandler(me()),
      jobs.handler(() => HttpResponse.json({ jobs: [job] })),
      ...appHandlers(),
    );
    renderAt("/settings/system/tasks");

    expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
    expect(screen.getByText("Server is restarting")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Tasks" })).not.toBeInTheDocument();

    jobs.state.failing = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Acquisitions")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();
  });
});

describe("Settings → Library", () => {
  const browse = http.get("*/v1/library/browse", () =>
    HttpResponse.json({ path: "", parent: "", inUse: false, holds: [], folders: [] }),
  );

  it("opens on its media folders", async () => {
    server.use(
      getMeMockHandler(me()),
      browse,
      getListLibraryRootsMockHandler({ roots: [folder], pathMap: [] }),
      ...appHandlers(),
    );
    renderAt("/settings/library");

    expect(await screen.findByText("/media/movies")).toBeInTheDocument();
    expect(screen.queryByText("Loading your media folders")).not.toBeInTheDocument();
  });

  it("shows a failed first read as the route's error view, and recovers on Try again", async () => {
    const roots = failing("*/v1/library/roots");
    server.use(
      getMeMockHandler(me()),
      browse,
      roots.handler(() => HttpResponse.json({ roots: [folder], pathMap: [] })),
      ...appHandlers(),
    );
    renderAt("/settings/library");

    expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
    expect(screen.getByText("Server is restarting")).toBeInTheDocument();

    roots.state.failing = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("/media/movies")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument());
  });
});
