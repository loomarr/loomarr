import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// Router-level auth coverage (§11, §19): the _authed beforeLoad guard + the login flow,
// driven through the REAL generated route tree. `me` flips to 200 once a login POST
// lands, so the sign-in test exercises the guard re-running on navigation.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const json = (body: unknown, status: number) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (startAuthed: boolean) => {
  let authed = startAuthed;
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/login")) {
      authed = true;
      return Promise.resolve(json(ADMIN, 200));
    }
    if (u.includes("/v1/auth/me")) {
      return Promise.resolve(authed ? json(ADMIN, 200) : json({ title: "Unauthorized" }, 401));
    }
    return Promise.resolve(json({}, 200));
  });
  vi.stubGlobal("fetch", mock);
  return mock;
};

const renderApp = (initialPath: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  // Returned so a test can assert WHERE the router settled — the only way to prove a redirect
  // actually navigated rather than leaving the old URL rendering nothing.
  return router;
};

afterEach(() => vi.restoreAllMocks());

describe("app router auth", () => {
  it("bounces a signed-out visitor from a protected route to the login form", async () => {
    stubFetch(false);
    renderApp("/guide");
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
  });

  it("renders the protected screen when already authenticated", async () => {
    stubFetch(true);
    renderApp("/guide");
    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("signs in and lands on the Channels home", async () => {
    const fetchMock = stubFetch(false);
    renderApp("/login");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
    const posted = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/auth/login"));
    expect(posted?.[1]).toMatchObject({ method: "POST", credentials: "include" });
  });
});

// LEGACY DEEP LINKS — the bookmark-compatibility promise (V-nav-paths).
//
// ⚠ These exist because the Filler redirect SHIPPED BROKEN and every unit test passed. It threw
// its `redirect` from `validateSearch`, where the router is still parsing the URL and treats the
// throw as a parse failure — so `/filler?tab=sources` rendered a BLANK page at the old address.
// Nothing caught it: no test asserted on the final URL, and rendering at the new path (which the
// reachability suite does) exercises a different code path entirely. Only loading the old URL in
// a browser showed it.
//
// The assertion is on where the router SETTLED, not on what rendered — a page can look fine and
// still be at the wrong address, and a redirect that does not move the URL is not a redirect.
describe("legacy tab links redirect to their new paths", () => {
  // ⚠ `location.href`, NOT `pathname + search`. TanStack types `location.search` as the PARSED
  // object, so concatenating it threw "Cannot convert object to primitive value" — on every
  // case, including `/queue`, which has no redirect at all. Five identical failures that all
  // pointed at the app were entirely this helper.
  const at = (router: ReturnType<typeof renderApp>) => router.state.location.href;

  it("/filler?tab=sources lands on /filler/sources", async () => {
    stubFetch(true);
    const router = renderApp("/filler?tab=sources");
    await waitFor(() => expect(at(router)).toBe("/filler/sources"));
  });

  it("/filler?tab=incoming lands on /filler/incoming", async () => {
    stubFetch(true);
    const router = renderApp("/filler?tab=incoming");
    await waitFor(() => expect(at(router)).toBe("/filler/incoming"));
  });

  // ⚠ The catalog FILTERS survive the move. They are query params on purpose — a shared link to
  // a searched view has to keep working — so a redirect that dropped them would silently change
  // what the recipient sees.
  it("carries the catalog filters through the redirect", async () => {
    stubFetch(true);
    const router = renderApp("/filler?tab=sources&q=coke&view=list");
    await waitFor(() => expect(at(router)).toContain("/filler/sources"));
    expect(at(router)).toContain("q=coke");
    expect(at(router)).toContain("view=list");
  });

  it("/channels/ch-1?section=filler lands on the filler section", async () => {
    stubFetch(true);
    const router = renderApp("/channels/ch-1?section=filler");
    await waitFor(() => expect(at(router)).toBe("/channels/ch-1/filler"));
  });

  // `/queue` has no legacy param to translate — it RESOLVES a default instead, which is the same
  // promise seen from the other side: a bare link must land somewhere real.
  it("/queue resolves to a real section", async () => {
    stubFetch(true);
    const router = renderApp("/queue");
    await waitFor(() => expect(at(router)).toMatch(/^\/queue\/(approval|flight)$/));
  });
});
