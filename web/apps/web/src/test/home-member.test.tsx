import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// A member's Home (#1659, Q-H1/Q-H2). It replaced V16's lockout: members get Home now, because
// it carries no machine state. What V16 guarded still holds, and is what this file proves: a
// member's Home never ASKS for the admin-only data. Hiding the results is not enough — firing the
// requests would put 403s in the console and briefly flash an error.

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

// Records every URL fetched, so the test can assert the admin-only endpoints were never even
// asked for.
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
      if (u.includes("/v1/guide")) return Promise.resolve(json({ channels: [], fromMs: 0, toMs: 0 }));
      // Anything admin-only answers 403, as the real API would — so a screen that ignored the
      // role and queried anyway would visibly fail rather than quietly pass.
      if (u.includes("/v1/system/services") || u.includes("/v1/system/restart")) {
        return Promise.resolve(json({}, 403));
      }
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

describe("Home — a member's view (#1659)", () => {
  it("shows Home with the member's call to action, not the admin's", async () => {
    stubAs(MEMBER);
    renderAt("/dashboard");

    expect(await screen.findByText("Your first channel starts here")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Request a channel" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add your first channel" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Settings → This server/)).not.toBeInTheDocument();
  });

  // Services, restart cost and the restart notice are admin-only (§11; #1659 Q-S1).
  it("never requests the admin-only server state", async () => {
    const seen = stubAs(MEMBER);
    renderAt("/dashboard");

    await screen.findByText("Your first channel starts here");
    await waitFor(() => {
      expect(seen.some((u) => u.includes("/v1/system/services") || u.includes("/v1/system/restart"))).toBe(
        false,
      );
    });
    expect(screen.queryByText(/Restart Loomarr/)).not.toBeInTheDocument();
  });
});
