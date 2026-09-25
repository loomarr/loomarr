import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePendingApprovals } from "./pending-approvals";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const stub = () => {
  const fetchSpy = vi.fn((url: string) => {
    if (url.includes("/v1/filler/pulls")) return Promise.resolve(jsonResponse({ pulls: [{ id: "pull_1" }] }));
    if (url.includes("/v1/proposals")) {
      return Promise.resolve(jsonResponse({ proposals: [{ id: "p1" }, { id: "p2" }] }));
    }
    return Promise.resolve(jsonResponse({}));
  });
  vi.stubGlobal("fetch", fetchSpy);
  return fetchSpy;
};

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

afterEach(() => vi.unstubAllGlobals());

describe("usePendingApprovals", () => {
  it("counts pending pulls together with submitted proposals", async () => {
    stub();
    const { result } = renderHook(() => usePendingApprovals(true), { wrapper });

    await waitFor(() => expect(result.current.count).toBe(3));
    expect(result.current.proposalCount).toBe(2);
    expect(result.current.pullCount).toBe(1);
  });

  it("fires no request for a caller who is not an admin", async () => {
    const fetchSpy = stub();
    const { result } = renderHook(() => usePendingApprovals(false), { wrapper });

    // Nothing to wait for — assert on the settled state after a tick.
    await new Promise((r) => setTimeout(r, 20));
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(result.current.count).toBe(0);
  });
});
