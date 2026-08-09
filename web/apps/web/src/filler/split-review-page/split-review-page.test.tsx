import type { MeBody } from "@loomarr/api";
import {
  getConfirmFillerSplitMockHandler,
  getGetFillerSplitMockHandler,
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
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { SplitReviewPage } from "./split-review-page";

// A MINIMAL router, not the app tree: the page navigates on confirm/back, and TanStack
// throws navigating to a route the tree doesn't have — so the tree is exactly the two
// paths this page can land on.
// ⚠ `local` is REQUIRED on MeBody and this fixture omitted it — the third file in this migration
// to carry the same incomplete user object, all of them invisible while the stub was untyped.
const ADMIN = {
  id: "u1",
  name: "Ada",
  role: "admin" as const,
  local: true,
  autoApprove: true,
  disabled: false,
  quota: 0,
};
const MEMBER = { ...ADMIN, role: "member" as const };

const PROPOSAL = {
  id: "sp-1",
  clipHash: "comp-hash",
  createdAt: "2026-07-25T20:00:00Z",
  segments: [
    { index: 0, startMs: 0, endMs: 30000, name: "First ad", era: 1990, audience: "kids", category: "toys" },
    { index: 1, startMs: 30000, endMs: 61000, name: "Second ad", suggestedEra: 1985 },
  ],
};

// ⚠ The stub this replaced dispatched by URL SUBSTRING, in order, with a catch-all `{}` at the
// end — and the ordering was load-bearing: `/v1/filler/splits/sp-1/confirm` had to be tested
// before `/v1/filler/splits/`, which had to precede `/v1/filler`. A prefix that also matches its
// own sibling is a bug waiting for someone to reorder the list. Route-bound handlers have no
// order at all.
//
// ⚠ `fetchedProposal` / `confirms` replace assertions over `fetchMock.mock.calls`. The NEGATIVE
// one matters most here (§19's member case): "the proposal was never fetched" used to mean "no
// recorded url contained that substring", which is only as good as the substring. Now the handler
// simply never fires — AND, if the member path ever did fetch something unmodelled, the
// unhandled-request guard fails the test by name rather than letting a catch-all answer it.
const stubSplit = (me: MeBody = ADMIN) => {
  let fetchedProposal = false;
  const confirms: unknown[] = [];
  server.use(
    getMeMockHandler({ ...me }),
    getGetFillerSplitMockHandler(() => {
      fetchedProposal = true;
      return PROPOSAL;
    }),
    getConfirmFillerSplitMockHandler(async ({ request }) => {
      confirms.push(await request.json());
      return { clips: 2 };
    }),
  );
  return { wasFetched: () => fetchedProposal, confirms };
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

describe("SplitReviewPage", () => {
  it("loads the persisted proposal and renders the cut list", async () => {
    stubSplit();
    renderPage();
    expect(await screen.findByRole("heading", { name: /review split/i })).toBeInTheDocument();
    expect(await screen.findByRole("region", { name: /segment 1: first ad/i })).toBeInTheDocument();
    expect(screen.getByText("comp-hash")).toBeInTheDocument();
  });

  it("confirms the edited draft as the POST body and returns to the catalog", async () => {
    const { confirms } = stubSplit();
    const router = renderPage();
    const second = await screen.findByRole("region", { name: /segment 2: second ad/i });
    // Answer the open era question, then commit.
    fireEvent.click(within(second).getByRole("button", { name: /accept 1985/i }));
    fireEvent.click(screen.getByRole("button", { name: /confirm cuts/i }));

    await screen.findByText("the catalog");
    expect(confirms, "confirm should POST the edited cut list").toHaveLength(1);
    const body = confirms[0] as { segments: Array<Record<string, unknown>> };
    expect(body.segments).toHaveLength(2);
    expect(body.segments[1]).toMatchObject({ index: 1, era: 1985 });
    expect(body.segments[1]?.suggestedEra).toBeUndefined();
    expect(router.state.location.pathname).toBe("/filler");
  });

  it("Back leaves without calling confirm", async () => {
    const { confirms } = stubSplit();
    renderPage();
    await screen.findByRole("region", { name: /segment 1: first ad/i });
    fireEvent.click(screen.getByRole("button", { name: /^back$/i }));
    await screen.findByText("the catalog");
    expect(confirms).toHaveLength(0);
  });

  // §19's negative half, on the UI side: a member gets the explanation, and the proposal
  // query never fires (the server would 403 it anyway — the gate keeps the console clean).
  it("explains rather than fetching for a member", async () => {
    const { wasFetched } = stubSplit(MEMBER);
    renderPage();
    expect(await screen.findByText(/admins only/i)).toBeInTheDocument();
    expect(wasFetched()).toBe(false);
  });
});
