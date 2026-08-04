import { fillerApi } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useFillerInvalidate } from "./use-filler-invalidate";

// The three filler query keys travel together for a reason that is easy to forget at a call
// site: filing, holding, removing or retagging a clip moves it between the Incoming queue and
// the Catalog, and both change what the pool strip reports. Invalidating only the clip list
// leaves the row sitting in Incoming until a reload — "it worked but the UI lied".
//
// ⚠ These tests assert the KEYS, not a call count. The bug this hook exists to prevent is a
// call site that invalidates two of the three, so "was invalidateQueries called" would pass
// against exactly the defect.

const wrapper = (client: QueryClient) => {
  const Wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return Wrapper;
};

const setup = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(client, "invalidateQueries").mockResolvedValue();
  const { result } = renderHook(() => useFillerInvalidate(), { wrapper: wrapper(client) });
  // Compared as JSON: the keys are nested arrays, so identity never matches and a bare
  // `toContain` would pass on a near-miss.
  const invalidated = () => spy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
  return { invalidated, result };
};

const CATALOG = JSON.stringify(fillerApi.getListFillerQueryKey());
const INCOMING = JSON.stringify(fillerApi.getFillerIncomingQueryKey());
const POOL = JSON.stringify(fillerApi.getFillerPoolQueryKey());

describe("useFillerInvalidate", () => {
  it("invalidateCatalog touches ONLY the clip list", () => {
    const { invalidated, result } = setup();
    result.current.invalidateCatalog();
    expect(invalidated()).toEqual([CATALOG]);
  });

  it("invalidateQueues touches Incoming and the pool, not the catalog", () => {
    const { invalidated, result } = setup();
    result.current.invalidateQueues();
    expect(invalidated()).toEqual(expect.arrayContaining([INCOMING, POOL]));
    expect(invalidated()).not.toContain(CATALOG);
  });

  // ⚠ THE one that matters. A lifecycle write can move a clip between all three views, so
  // missing any single key shows the operator a stale queue. Asserted as a set so the hook is
  // free to reorder, but nothing may drop out.
  it("invalidateLifecycle touches all three, because a filed clip changes all three", () => {
    const { invalidated, result } = setup();
    result.current.invalidateLifecycle();
    expect(invalidated().sort()).toEqual([CATALOG, INCOMING, POOL].sort());
  });
});
