import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// Shared by the Requests specs (#1405): the real route tree, driven the way the Dashboard cards,
// the tab bar, the nav badge and a member's browser reach it. Each colocated spec owns its own
// assertions; this owns the fixtures and the stubs so they cannot drift apart.

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

const journey = (jobId: string, over: Record<string, unknown> = {}) => ({
  version: 1,
  jobId,
  milestone: "live",
  intent: { description: `Request ${jobId}` },
  attempts: [],
  actions: [],
  createdAt: "2026-09-24T18:00:00Z",
  updatedAt: "2026-09-24T18:00:00Z",
  ...over,
});

const failed = (jobId: string) =>
  journey(jobId, {
    milestone: "failed",
    actions: ["retry"],
    failure: {
      code: "generation_failed",
      reason: "provider_timeout",
      recoveryAction: "retry_later",
      message: "The model took too long.",
      guidance: "Try again in a moment.",
    },
  });

/** An approved request whose proposal asks Loomarr to acquire `acquisitions`. */
const approved = (jobId: string, milestone: string, acquisitions: unknown[] = []) =>
  journey(jobId, {
    milestone,
    proposal: {
      id: `p-${jobId}`,
      status: "approved",
      approvedByName: "Ada",
      proposal: { intent: { description: "x" }, lineup: [], acquisitions },
    },
  });

const title = (key: string, state: string, over: Record<string, unknown> = {}) => ({
  key,
  mediaType: "movie",
  name: key,
  state,
  ...over,
});

type Fixture = {
  me: unknown;
  journeys?: unknown[];
  proposals?: unknown[];
  pulls?: unknown[];
  titles?: Record<string, unknown[]>;
};

// Records the URLs asked for, so a test can assert what a role never requested. Only the
// endpoints these tests assert on are overridden; `appHandlers` (spread LAST, see its note)
// answers the rest of what the real route tree fetches with empty-but-valid data.
const stub = ({ me, journeys = [], proposals = [], pulls = [], titles = {} }: Fixture) => {
  const seen: string[] = [];
  const path = (url: string) => url.replace(/^https?:\/\/[^/]+/, "");
  server.use(
    http.get("*/v1/auth/me", () => HttpResponse.json(me as never)),
    http.get("*/v1/proposal-jobs", ({ request }) => {
      seen.push(path(request.url));
      return HttpResponse.json({ journeys } as never);
    }),
    http.get("*/v1/proposal-jobs/:jobId", ({ params }) => {
      const found = (journeys as { jobId: string }[]).find((j) => j.jobId === params.jobId);
      return found
        ? HttpResponse.json(found as never)
        : HttpResponse.json({ title: "Not found", status: 404 }, { status: 404 });
    }),
    http.get("*/v1/proposals", ({ request }) => {
      seen.push(path(request.url));
      const status = new URL(request.url).searchParams.get("status");
      return HttpResponse.json({ proposals: status === "submitted" ? proposals : [] } as never);
    }),
    http.get("*/v1/filler/pulls", () => HttpResponse.json({ pulls } as never)),
    http.get("*/v1/titles", ({ request }) => {
      const state = new URL(request.url).searchParams.get("state") ?? "";
      return HttpResponse.json({ titles: titles[state] ?? [] } as never);
    }),
    // Read by the Dashboard / approval cards these specs mount and not part of the baseline.
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

export { ADMIN, approved, failed, journey, MEMBER, proposal, renderAt, stub, title };
