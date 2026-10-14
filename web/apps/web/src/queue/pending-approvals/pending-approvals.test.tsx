import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { usePendingApprovals } from "./pending-approvals";

// The shared MSW layer (V53e) rather than a hand-rolled fetch stub: the handlers are bound to the
// real routes, and `seen` records which of them the hook actually called.
const stub = () => {
  const seen: string[] = [];
  server.use(
    http.get("*/v1/filler/pulls", ({ request }) => {
      seen.push(request.url);
      return HttpResponse.json({ pulls: [{ id: "pull_1" }] } as never);
    }),
    http.get("*/v1/proposals", ({ request }) => {
      seen.push(request.url);
      return HttpResponse.json({ proposals: [{ id: "p1" }, { id: "p2" }] } as never);
    }),
    ...appHandlers(),
  );
  return seen;
};

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("usePendingApprovals", () => {
  it("counts pending pulls together with submitted proposals", async () => {
    stub();
    const { result } = renderHook(() => usePendingApprovals(true), { wrapper });

    await waitFor(() => expect(result.current.count).toBe(3));
    expect(result.current.proposalCount).toBe(2);
    expect(result.current.pullCount).toBe(1);
  });

  it("fires no request for a caller who is not an admin", async () => {
    const seen = stub();
    const { result } = renderHook(() => usePendingApprovals(false), { wrapper });

    // Nothing to wait for — assert on the settled state after a tick.
    await new Promise((r) => setTimeout(r, 20));
    expect(seen).toEqual([]);
    expect(result.current.count).toBe(0);
  });
});
