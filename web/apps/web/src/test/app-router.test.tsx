import type { MeBody } from "@loomarr/api";
import { getLoginMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// Router-level auth coverage (§11, §19): the _authed beforeLoad guard + the login flow,
// driven through the REAL generated route tree. `me` flips to 200 once a login POST
// lands, so the sign-in test exercises the guard re-running on navigation.
//
// ⚠ `local` is REQUIRED on MeBody and the old fixture omitted it — the fourth file in this
// migration carrying that same incomplete user.
const ADMIN: MeBody = {
  id: "u1",
  name: "Ada",
  role: "admin",
  local: true,
  autoApprove: true,
  disabled: false,
  quota: 0,
};

// ⚠ THE POINT OF `appHandlers`: this test mounts the REAL route tree, so the app fetches whatever
// each landed route needs. The stub this replaced answered ALL of that with a catch-all
// `json({}, 200)` — every screen in the suite rendered against empty objects and nothing said so.
// `appHandlers` is the shared baseline (see src/test/msw/handlers.ts); this file adds only the
// auth behaviour it is actually testing.
//
// `me` is hand-written because it is STATEFUL (401 → 200 after login) and status-bearing, which
// generated handlers cannot express — the spec declares errors via `default:` with no 401 code.
const stubAuth = (startAuthed: boolean) => {
  let authed = startAuthed;
  const logins: unknown[] = [];
  server.use(
    ...appHandlers(),
    http.get("*/v1/auth/me", () =>
      authed ? HttpResponse.json(ADMIN) : HttpResponse.json({ title: "Unauthorized" }, { status: 401 }),
    ),
    getLoginMockHandler(async ({ request }) => {
      logins.push(await request.json());
      authed = true;
      return ADMIN;
    }),
  );
  return { logins };
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

describe("app router auth", () => {
  it("bounces a signed-out visitor from a protected route to the login form", async () => {
    stubAuth(false);
    renderApp("/guide");
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
  });

  it("renders the protected screen when already authenticated", async () => {
    stubAuth(true);
    renderApp("/guide");
    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("signs in and lands on the Channels home", async () => {
    const { logins } = stubAuth(false);
    renderApp("/login");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
    // ⚠ The old assertion dug into `fetchMock.mock.calls` for a url SUBSTRING and then checked
    // `{ method: "POST", credentials: "include" }` — i.e. it asserted the TEST STUB was called a
    // certain way. Reaching the route-bound login resolver is the stronger claim; what remains
    // worth asserting is the credential payload the form actually sent.
    expect(logins).toEqual([{ username: "ada", password: "hunter2!" }]);
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
    stubAuth(true);
    const router = renderApp("/filler?tab=sources");
    await waitFor(() => expect(at(router)).toBe("/filler/sources"));
  });

  it("/filler?tab=incoming lands on /filler/incoming", async () => {
    stubAuth(true);
    const router = renderApp("/filler?tab=incoming");
    await waitFor(() => expect(at(router)).toBe("/filler/incoming"));
  });

  // ⚠ The catalog FILTERS survive the move. They are query params on purpose — a shared link to
  // a searched view has to keep working — so a redirect that dropped them would silently change
  // what the recipient sees.
  it("carries the catalog filters through the redirect", async () => {
    stubAuth(true);
    const router = renderApp("/filler?tab=sources&q=coke&view=list");
    await waitFor(() => expect(at(router)).toContain("/filler/sources"));
    expect(at(router)).toContain("q=coke");
    expect(at(router)).toContain("view=list");
  });

  it("/channels/ch-1?section=filler lands on the filler section", async () => {
    stubAuth(true);
    const router = renderApp("/channels/ch-1?section=filler");
    await waitFor(() => expect(at(router)).toBe("/channels/ch-1/filler"));
  });

  // `/queue` has no legacy param to translate — it RESOLVES a default instead, which is the same
  // promise seen from the other side: a bare link must land somewhere real.
  it("/queue resolves to a real section", async () => {
    stubAuth(true);
    const router = renderApp("/queue");
    await waitFor(() => expect(at(router)).toMatch(/^\/queue\/(approval|flight)$/));
  });
});
