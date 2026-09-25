import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// #1404 — the Queue page's confirmed defects, driven through the real router the way the
// Dashboard cards, the tab bar and a member's browser reach them.

const ADMIN = {
  id: "u1",
  name: "Ada",
  role: "admin",
  autoApprove: false,
  disabled: false,
  quota: 0,
  local: true,
};
const MEMBER = { ...ADMIN, id: "u2", name: "Bo", role: "member", quota: 5 };

const proposal = {
  id: "p1",
  jobId: "j1",
  status: "submitted",
  createdBy: "u2",
  createdByName: "Bo",
  proposal: { intent: { description: "Saturday cartoons" }, lineup: [], acquisitions: [] },
};

type Fixture = {
  me: unknown;
  proposals?: unknown[];
  pulls?: unknown[];
  titles?: Record<string, unknown[]>;
};

// Records the proposal URLs asked for, so a test can assert what a role never requested. Only
// the endpoints these tests assert on are overridden; `appHandlers` (spread LAST, see its note)
// answers the rest of what the real route tree fetches with empty-but-valid data.
const stub = ({ me, proposals = [], pulls = [], titles = {} }: Fixture) => {
  const seen: string[] = [];
  server.use(
    http.get("*/v1/auth/me", () => HttpResponse.json(me as never)),
    http.get("*/v1/proposals", ({ request }) => {
      seen.push(request.url.replace(/^https?:\/\/[^/]+/, ""));
      const status = new URL(request.url).searchParams.get("status");
      return HttpResponse.json({ proposals: status === "submitted" ? proposals : [] } as never);
    }),
    http.get("*/v1/filler/pulls", () => HttpResponse.json({ pulls } as never)),
    http.get("*/v1/titles", ({ request }) => {
      const state = new URL(request.url).searchParams.get("state") ?? "";
      return HttpResponse.json({ titles: titles[state] ?? [] } as never);
    }),
    // Read by the Dashboard / approval cards this suite mounts and not part of the baseline.
    http.get("*/v1/discovery/feedback", () => HttpResponse.json([] as never)),
    http.post("*/v1/proposals/:id/outlook", () => HttpResponse.json({}, { status: 404 })),
    http.get("*/v1/playout/status", () => HttpResponse.json({}, { status: 404 })),
    http.get("*/v1/system/restart", () => HttpResponse.json({}, { status: 404 })),
    http.get("*/v1/system/services", () => HttpResponse.json({}, { status: 404 })),
    ...appHandlers(),
  );
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  return seen;
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

const title = (key: string, state: string, over: Record<string, unknown> = {}) => ({
  key,
  mediaType: "movie",
  name: key,
  state,
  ...over,
});

afterEach(() => vi.restoreAllMocks());

describe("Dashboard cards link to the tab they name", () => {
  it("'Needs you' lands on Needs approval and 'Acquiring' on In flight", async () => {
    stub({ me: ADMIN, proposals: [proposal] });
    const router = renderAt("/dashboard");

    await userEvent.click(await screen.findByRole("link", { name: /Acquiring/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/queue/flight"));

    await router.navigate({ to: "/dashboard" });
    await userEvent.click(await screen.findByRole("link", { name: /Needs you/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/queue/approval"));
  });

  it("'Acquiring' counts only what is in flight, not landed or given-up titles", async () => {
    stub({
      me: ADMIN,
      titles: {
        wanted: [title("a", "wanted")],
        requested: [title("b", "requested")],
        downloading: [title("c", "downloading")],
        available: [title("d", "available")],
        unavailable: [title("e", "unavailable", { lastError: "deadline exceeded" })],
      },
    });
    renderAt("/dashboard");

    const card = await screen.findByRole("link", { name: /Acquiring/ });
    await waitFor(() => expect(within(card).getByText("3")).toBeInTheDocument());
  });
});

describe("Queue tabs", () => {
  it("'In flight' counts wanted/requested/downloading only", async () => {
    stub({
      me: ADMIN,
      titles: {
        wanted: [title("a", "wanted")],
        available: [title("d", "available"), title("f", "available")],
        unavailable: [title("e", "unavailable")],
      },
    });
    renderAt("/queue/flight");

    const tab = await screen.findByRole("link", { name: /In flight/ });
    await waitFor(() => expect(tab).toHaveTextContent(/In flight\s*1\b/));
  });

  it("describes a given-up title as given up, with why, not as queued", async () => {
    stub({
      me: ADMIN,
      titles: { unavailable: [title("Gave Up", "unavailable", { lastError: "deadline exceeded" })] },
    });
    renderAt("/queue/flight");

    expect(await screen.findByText(/deadline exceeded/)).toBeInTheDocument();
    expect(screen.queryByText(/Approved and queued/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Try again/ })).toBeInTheDocument();
  });

  it("counts a pending pull towards Needs approval", async () => {
    stub({ me: ADMIN, pulls: [{ id: "pull1", status: "pending", items: [] }] });
    renderAt("/queue/approval");

    const tab = await screen.findByRole("link", { name: /Needs approval/ });
    await waitFor(() => expect(tab).toHaveTextContent(/Needs approval\s*1\b/));
  });
});

describe("Members on the Queue", () => {
  it("see no Try again button on a given-up title", async () => {
    stub({ me: MEMBER, titles: { unavailable: [title("Gave Up", "unavailable")] } });
    renderAt("/queue/flight");

    await screen.findByText("Gave Up");
    expect(screen.queryByRole("button", { name: /Try again/ })).not.toBeInTheDocument();
  });

  it("never fetch everyone's decided proposals", async () => {
    const seen = stub({ me: MEMBER });
    renderAt("/queue/flight");

    await screen.findByRole("link", { name: /In flight/ });
    expect(seen.filter((u) => /status=(approved|denied)$/.test(u))).toEqual([]);
  });

  it.each(["/queue/approval", "/queue/history"])("are redirected away from %s", async (path) => {
    const seen = stub({ me: MEMBER, proposals: [proposal] });
    const router = renderAt(path);

    await waitFor(() => expect(router.state.location.pathname).toBe("/queue/flight"));
    expect(seen.some((u) => /status=submitted$/.test(u))).toBe(false);
  });
});
