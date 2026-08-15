import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// V16's gate, in its own words: "member sees the lockout, not a 403 wall."
//
// The reachability suite cannot cover this — it runs as an ADMIN and asks "does every route
// render real content", which is a different question from "what does a member see". So the
// lockout had code and an assertion in the PR description, and nothing proving it.
//
// Why it matters beyond politeness: the dashboard's data is admin-gated server-side, so a
// member who lands here would otherwise fire four requests that all 403, and see either an
// error state or an empty shell. Neither says "this isn't for you, here's where to go" — and
// the member did nothing wrong.

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const MEMBER = {
  id: "u2",
  name: "Bo",
  role: "member",
  autoApprove: false,
  disabled: false,
  quota: 5,
  local: true,
};

const ADMIN = { ...MEMBER, id: "u1", name: "Ada", role: "admin" };

// Records every URL fetched, so the test can assert the admin-only endpoints were never even
// asked for — a lockout that still fires the requests is a lockout in appearance only.
const stubAs = (me: unknown) => {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const u = String(input);
      seen.push(u);
      if (u.includes("/v1/auth/me")) return Promise.resolve(json(me));
      if (u.includes("/v1/setup/status")) {
        return Promise.resolve(json({ ready: true, bootstrapped: true, checks: [] }));
      }
      if (u.includes("/v1/settings")) return Promise.resolve(json({ features: {}, settings: [] }));
      if (u.includes("/v1/dashboard/summary")) {
        return Promise.resolve(
          json({
            onAir: 2,
            channels: 7,
            needsApproval: 3,
            acquiring: 4,
            unavailable: 1,
            generatedAt: Date.now(),
          }),
        );
      }
      if (u.includes("/v1/playout/status")) {
        return Promise.resolve(json({ running: false, gpu: {}, channels: [] }));
      }
      if (u.includes("/v1/system/services")) {
        return Promise.resolve(json({ loomarr: { name: "loomarr", ok: true }, rows: [] }));
      }
      if (u.includes("/v1/system/restart")) {
        return Promise.resolve(json({ available: false, streamingChannels: 0, pendingKeys: [] }));
      }
      if (u.includes("/v1/activity")) return Promise.resolve(json({ activity: [] }));
      // Anything admin-only answers 403, as the real API would — so a screen that ignored the
      // role and queried anyway would visibly fail rather than quietly pass.
      if (u.includes("/v1/playout/status")) return Promise.resolve(json({}, 403));
      return Promise.resolve(json({}));
    }),
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
};

afterEach(() => vi.restoreAllMocks());

describe("Dashboard — member lockout (V16)", () => {
  it("explains the screen instead of showing an error", async () => {
    stubAs(MEMBER);
    renderAt("/dashboard");

    const lockout = await screen.findByText(/dashboard is for admins/i);
    expect(lockout).toBeInTheDocument();
    // And it points somewhere useful rather than dead-ending. Scoped to the lockout card:
    // "My requests" is also the member's nav label, so an unscoped query matches both.
    expect(lockout.parentElement).toHaveTextContent(/My requests/);
  });

  // ⚠ The lockout must PREVENT the requests, not just hide their results. Firing them anyway
  // would put 403s in the console on a screen we deliberately show instead, and would make the
  // panel briefly flash an error before the lockout won.
  it("never requests the admin-only telemetry", async () => {
    const seen = stubAs(MEMBER);
    renderAt("/dashboard");

    await screen.findByText(/dashboard is for admins/i);
    await waitFor(() => {
      expect(
        seen.some((u) =>
          ["/v1/dashboard/summary", "/v1/playout/status", "/v1/system/services", "/v1/activity"].some(
            (path) => u.includes(path),
          ),
        ),
      ).toBe(false);
    });
  });

  it("uses one operational summary instead of downloading full collections", async () => {
    const seen = stubAs(ADMIN);
    renderAt("/dashboard");

    expect(await screen.findByText("of 7 managed channels")).toBeInTheDocument();
    expect(screen.getByText("requests waiting on approval")).toBeInTheDocument();
    expect(screen.getByText("titles that need attention")).toBeInTheDocument();
    expect(seen.some((u) => u.includes("/v1/dashboard/summary"))).toBe(true);
    expect(
      seen.some((u) => ["/v1/channels", "/v1/titles", "/v1/proposals"].some((path) => u.includes(path))),
    ).toBe(false);
  });
});
