import { getGetCurrentHealthMockHandler } from "@loomarr/api/msw";
import type { HealthReport } from "@loomarr/api/models/healthReport";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useCurrentHealth } from "./use-current-health";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

// The health report, whose next refresh is `after` from now (0: none); each read is counted.
const serve = (after: number) => {
  const reads = { count: 0 };
  server.use(
    getGetCurrentHealthMockHandler(() => {
      reads.count += 1;
      return {
        checks: [],
        nextRefreshAt: after ? Date.now() + after : 0,
        state: "healthy",
      } as unknown as HealthReport;
    }),
  );
  return reads;
};

describe("useCurrentHealth", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => vi.useRealTimers());

  it("reads again once a check's result can have run out, and not before", async () => {
    const reads = serve(60_000);
    renderHook(() => useCurrentHealth(), { wrapper });
    await waitFor(() => expect(reads.count).toBe(1));

    await act(() => vi.advanceTimersByTimeAsync(55_000));
    expect(reads.count).toBe(1);

    await act(() => vi.advanceTimersByTimeAsync(10_000));
    await waitFor(() => expect(reads.count).toBe(2));
  });

  it("never polls a report that nothing can make stale", async () => {
    const reads = serve(0);
    renderHook(() => useCurrentHealth(), { wrapper });
    await waitFor(() => expect(reads.count).toBe(1));

    await act(() => vi.advanceTimersByTimeAsync(10 * 60_000));
    expect(reads.count).toBe(1);
  });

  it("reads nothing while disabled", async () => {
    const reads = serve(1_000);
    renderHook(() => useCurrentHealth({ enabled: false }), { wrapper });

    await act(() => vi.advanceTimersByTimeAsync(5_000));
    expect(reads.count).toBe(0);
  });
});
