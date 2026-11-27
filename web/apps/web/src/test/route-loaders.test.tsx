import type { JobView } from "@loomarr/api";
import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as libraryApi from "@loomarr/api/endpoints/library";
import {
  getFillerWatchMockHandler,
  getJobsListMockHandler,
  getListInvitationsMockHandler,
  getListLibraryRootsMockHandler,
  getListUsersMockHandler,
  getMeMockHandler,
  getSettingsListMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RouteError } from "@/components/loomarr/feedback/route-error";
import { routeTree } from "@/routeTree.gen";
import { fillerSource, readiness } from "@/test/fixtures/filler";
import { me, user } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";

// A page whose loader ensures the data it is made of (#3399 step 9.2): it opens on that data, and a
// first read that fails is the route's error view, with a retry. A refresh that fails once the page
// is open is the page's own error, beside the data it already has.

const clients: QueryClient[] = [];
afterEach(async () => {
  const done = clients.splice(0);
  await Promise.all(done.map((c) => vi.waitFor(() => expect(c.isFetching()).toBe(0))));
  for (const c of done) c.clear();
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
  return queryClient;
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

const failing = (endpoint: string, initially = true) => {
  const state = { failing: initially };
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

  it("shows a refresh that fails once the page is open as the page's own error, with a retry", async () => {
    const jobs = failing("*/v1/jobs", false);
    server.use(
      getMeMockHandler(me()),
      jobs.handler(() => HttpResponse.json({ jobs: [job] })),
      ...appHandlers(),
    );
    const queryClient = renderAt("/settings/system/tasks");
    expect(await screen.findByText("Acquisitions")).toBeInTheDocument();

    jobs.state.failing = true;
    await act(() => queryClient.invalidateQueries({ queryKey: jobsApi.getJobsListQueryKey() }));

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Server is restarting")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();

    jobs.state.failing = false;
    await userEvent.click(within(alert).getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Acquisitions")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Server is restarting")).not.toBeInTheDocument());
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

  it("shows a refresh that fails once the page is open above the folders it already has", async () => {
    const roots = failing("*/v1/library/roots", false);
    server.use(
      getMeMockHandler(me()),
      browse,
      roots.handler(() => HttpResponse.json({ roots: [folder], pathMap: [] })),
      ...appHandlers(),
    );
    const queryClient = renderAt("/settings/library");
    expect(await screen.findByText("/media/movies")).toBeInTheDocument();

    roots.state.failing = true;
    await act(() => queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryRootsQueryKey() }));

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Server is restarting")).toBeInTheDocument();
    expect(screen.getByText("/media/movies")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();
  });
});

describe("People", () => {
  const people = [user({ id: "u1", name: "Ada" }), user({ id: "u2", name: "Grace", role: "member" })];

  it("opens on its people", async () => {
    server.use(
      getMeMockHandler(me()),
      getListUsersMockHandler({ users: people }),
      getListInvitationsMockHandler({ invitations: [] }),
      ...appHandlers(),
    );
    renderAt("/people");

    expect(await screen.findByText("Grace")).toBeInTheDocument();
  });

  it("shows a failed first read as the route's error view, and recovers on Try again", async () => {
    const users = failing("*/v1/users");
    server.use(
      getMeMockHandler(me()),
      users.handler(() => HttpResponse.json({ users: people })),
      getListInvitationsMockHandler({ invitations: [] }),
      ...appHandlers(),
    );
    renderAt("/people");

    expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
    expect(screen.getByText("Server is restarting")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "People" })).not.toBeInTheDocument();

    users.state.failing = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Grace")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument());
  });

  it("shows a failed invitations read as the route's error view", async () => {
    const invitations = failing("*/v1/invitations");
    server.use(
      getMeMockHandler(me()),
      getListUsersMockHandler({ users: people }),
      invitations.handler(() => HttpResponse.json({ invitations: [] })),
      ...appHandlers(),
    );
    renderAt("/people");

    expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
    expect(screen.queryByText("Grace")).not.toBeInTheDocument();
  });

  it("never asks a member for the people or the invitations", async () => {
    const asked: string[] = [];
    server.use(
      getMeMockHandler(me({ role: "member" })),
      http.get("*/v1/users", () => {
        asked.push("users");
        return HttpResponse.json({ users: people });
      }),
      http.get("*/v1/invitations", () => {
        asked.push("invitations");
        return HttpResponse.json({ invitations: [] });
      }),
      ...appHandlers(),
    );
    renderAt("/people");

    // The route has settled once the member's view of it renders, and its loader has run by then.
    expect(await screen.findByText("Admins only")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();
    expect(asked).toEqual([]);
  });
});

describe("Filler", () => {
  // An install with filler set up, which is what lets an admin's page past the "no filler folder
  // configured" state.
  const configured = { settings: [], features: { filler: true } };
  const proposal = {
    id: "sp-1",
    clipHash: "comp-hash",
    createdAt: "2026-07-25T20:00:00Z",
    checkedLanguages: ["en"],
    currentLanguages: ["en"],
    languageExclusions: [],
    segments: [{ index: 0, startMs: 0, endMs: 30000, name: "First ad", languageChecked: true }],
  };

  // The reads only an admin may make, recorded so a member's visit can be shown to send none.
  const adminReads = () => {
    const asked: string[] = [];
    const handlers = [
      ["settings", "*/v1/settings"],
      ["incoming", "*/v1/filler/incoming"],
      ["sources", "*/v1/filler/sources"],
      ["split", "*/v1/filler/splits/:id"],
      ["defects", "*/v1/filler/defects"],
    ].map(([name = "", url = ""]) =>
      http.get(url, () => {
        asked.push(name);
        return HttpResponse.json({ title: "Forbidden" }, { status: 403 });
      }),
    );
    return { asked, handlers };
  };

  const pages = [
    {
      name: "Overview",
      path: "/filler",
      endpoint: "*/v1/filler/readiness",
      body: readiness(),
      opened: "Channel coverage",
    },
    {
      name: "Incoming",
      path: "/filler/incoming",
      endpoint: "*/v1/filler/incoming",
      body: {
        preparing: { rows: [], total: 0 },
        needsHelp: { rows: [], total: 0 },
        recentlyReady: { rows: [], total: 0 },
        readyWindowSeconds: 604800,
      },
      opened: "Nothing on the way",
    },
    {
      name: "Taxonomy",
      path: "/filler/taxonomy",
      endpoint: "*/v1/taxonomy",
      body: { taxa: [], totalClips: 0, taggedClips: 0, unclassifiedClips: 0, axisCoverage: [] },
      opened: "Classification coverage",
    },
    {
      name: "Sources",
      path: "/filler/sources",
      endpoint: "*/v1/filler/sources",
      body: { sources: [fillerSource({ target: "/data/commercials" })], total: 1 },
      opened: "/data/commercials",
    },
    {
      name: "Split review",
      path: "/filler/splits/sp-1",
      endpoint: "*/v1/filler/splits/:id",
      body: proposal,
      opened: "Review clips",
    },
  ];

  describe.each(pages)("$name", ({ path, endpoint, body, opened }) => {
    it("opens on its data", async () => {
      server.use(
        getMeMockHandler(me()),
        getSettingsListMockHandler(configured),
        http.get(endpoint, () => HttpResponse.json(body)),
        ...appHandlers(),
      );
      renderAt(path);

      expect(await screen.findByText(opened)).toBeInTheDocument();
      expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();
    });

    it("shows a failed first read as the route's error view, and recovers on Try again", async () => {
      const read = failing(endpoint);
      server.use(
        getMeMockHandler(me()),
        getSettingsListMockHandler(configured),
        read.handler(() => HttpResponse.json(body)),
        ...appHandlers(),
      );
      renderAt(path);

      expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
      expect(screen.getByText("Server is restarting")).toBeInTheDocument();
      expect(screen.queryByText(opened)).not.toBeInTheDocument();

      read.state.failing = false;
      await userEvent.click(screen.getByRole("button", { name: "Try again" }));
      expect(await screen.findByText(opened)).toBeInTheDocument();
      await waitFor(() => expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument());
    });
  });

  // Incoming has no pending state of its own, so its page may not open before the read answers.
  it("holds Incoming back until what is on the way has been read", async () => {
    let answer = () => {};
    const held = new Promise<void>((resolve) => {
      answer = resolve;
    });
    let asked = false;
    server.use(
      getMeMockHandler(me()),
      getSettingsListMockHandler(configured),
      http.get("*/v1/filler/incoming", async () => {
        asked = true;
        await held;
        return HttpResponse.json(pages[1]?.body);
      }),
      ...appHandlers(),
    );
    renderAt("/filler/incoming");

    await waitFor(() => expect(asked).toBe(true));
    expect(screen.queryByRole("navigation", { name: "Filler sections" })).not.toBeInTheDocument();

    answer();
    expect(await screen.findByText("Nothing on the way")).toBeInTheDocument();
  });

  // What says whether filler is set up: the settings list for an admin, the watch summary for a
  // member. Every Filler page is drawn around it.
  it.each([
    { role: "admin" as const, path: "/filler/library", endpoint: "*/v1/settings" },
    { role: "admin" as const, path: "/filler/settings/storage", endpoint: "*/v1/settings" },
    { role: "member" as const, path: "/filler/library", endpoint: "*/v1/filler/watch" },
  ])(
    "shows $path as the route's error view when the $role's $endpoint read fails",
    async ({ role, path, endpoint }) => {
      const read = failing(endpoint);
      server.use(
        getMeMockHandler(me({ role })),
        read.handler(() =>
          HttpResponse.json(
            endpoint === "*/v1/settings"
              ? configured
              : { clips: 3, health: "healthy", preparing: 0, needsYou: 0, sourcesOn: 1, sourcesTotal: 1 },
          ),
        ),
        getSettingsListMockHandler(configured),
        ...appHandlers(),
      );
      renderAt(path);

      expect(await screen.findByText("Couldn't load this page")).toBeInTheDocument();
      expect(screen.queryByText("No filler folder configured")).not.toBeInTheDocument();

      read.state.failing = false;
      await userEvent.click(screen.getByRole("button", { name: "Try again" }));
      await waitFor(() => expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument());
      expect(await screen.findByRole("navigation", { name: "Filler sections" })).toBeInTheDocument();
    },
  );

  // Each waits for the member's own view before saying nothing was asked: the loader has run by then.
  it.each([
    { path: "/filler", view: "Channel coverage" },
    { path: "/filler/taxonomy", view: "Classification coverage" },
    { path: "/filler/incoming", view: "No clips yet" },
    { path: "/filler/sources", view: "No clips yet" },
    { path: "/filler/splits/sp-1", view: "Admins only" },
  ])("asks a member at $path for no admin data", async ({ path, view }) => {
    const { asked, handlers } = adminReads();
    server.use(
      getMeMockHandler(me({ role: "member" })),
      getFillerWatchMockHandler({
        clips: 3,
        health: "healthy",
        preparing: 0,
        needsYou: 0,
        sourcesOn: 1,
        sourcesReady: 1,
        sourcesTotal: 1,
      }),
      ...handlers,
      ...appHandlers(),
    );
    renderAt(path);

    expect(await screen.findByText(view)).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this page")).not.toBeInTheDocument();
    expect(asked).toEqual([]);
  });
});
