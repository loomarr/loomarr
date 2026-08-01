import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SplitReviewPage } from "./split-review-page";

// A MINIMAL router, not the app tree: the page navigates on confirm/back, and TanStack
// throws navigating to a route the tree doesn't have — so the tree is exactly the two
// paths this page can land on.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const MEMBER = { ...ADMIN, role: "member" };

const PROPOSAL = {
  id: "sp-1",
  clipPath: "comp.mp4",
  createdAt: "2026-07-25T20:00:00Z",
  segments: [
    { index: 0, startMs: 0, endMs: 30000, name: "First ad", era: 1990, audience: "kids", category: "toys" },
    { index: 1, startMs: 30000, endMs: 61000, name: "Second ad", suggestedEra: 1985 },
  ],
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (me: unknown = ADMIN) => {
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(me));
    if (u.includes("/v1/filler/splits/sp-1/confirm")) return Promise.resolve(json({ clips: 2 }));
    if (u.includes("/v1/filler/splits/")) return Promise.resolve(json(PROPOSAL));
    if (u.includes("/v1/filler")) return Promise.resolve(json({ clips: [] }));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  return mock;
};

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rootRoute = createRootRoute({ component: Outlet });
  const reviewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/filler/splits/$proposalId",
    component: () => <SplitReviewPage proposalId="sp-1" />,
  });
  const fillerRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/filler",
    component: () => <p>the catalog</p>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([reviewRoute, fillerRoute]),
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ["/filler/splits/sp-1"] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
};

afterEach(() => vi.restoreAllMocks());

describe("SplitReviewPage", () => {
  it("loads the persisted proposal and renders the cut list", async () => {
    stubFetch();
    renderPage();
    expect(await screen.findByRole("heading", { name: /review split/i })).toBeInTheDocument();
    expect(await screen.findByRole("region", { name: /segment 1: first ad/i })).toBeInTheDocument();
    expect(screen.getByText("comp.mp4")).toBeInTheDocument();
  });

  it("confirms the edited draft as the POST body and returns to the catalog", async () => {
    const fetchMock = stubFetch();
    const router = renderPage();
    const second = await screen.findByRole("region", { name: /segment 2: second ad/i });
    // Answer the open era question, then commit.
    fireEvent.click(within(second).getByRole("button", { name: /accept 1985/i }));
    fireEvent.click(screen.getByRole("button", { name: /confirm cuts/i }));

    await screen.findByText("the catalog");
    const posted = fetchMock.mock.calls.find(([u]) => String(u).includes("/confirm"));
    expect(posted, "confirm should POST the edited cut list").toBeDefined();
    const body = JSON.parse(String(posted?.[1]?.body)) as { segments: Array<Record<string, unknown>> };
    expect(body.segments).toHaveLength(2);
    expect(body.segments[1]).toMatchObject({ index: 1, era: 1985 });
    expect(body.segments[1]?.suggestedEra).toBeUndefined();
    expect(router.state.location.pathname).toBe("/filler");
  });

  it("Back leaves without calling confirm", async () => {
    const fetchMock = stubFetch();
    renderPage();
    await screen.findByRole("region", { name: /segment 1: first ad/i });
    fireEvent.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByText("the catalog");
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/confirm"))).toBe(false);
  });

  // §19's negative half, on the UI side: a member gets the explanation, and the proposal
  // query never fires (the server would 403 it anyway — the gate keeps the console clean).
  it("explains rather than fetching for a member", async () => {
    const fetchMock = stubFetch(MEMBER);
    renderPage();
    expect(await screen.findByText(/admins only/i)).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/filler/splits/"))).toBe(false);
  });
});
