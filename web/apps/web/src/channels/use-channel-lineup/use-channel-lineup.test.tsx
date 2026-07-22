import type { LineupEntryDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useChannelLineup } from "./use-channel-lineup";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Captures every PATCH body sent to /v1/channels/{id} so a test can assert exactly what
// the whole-list replace contained, in order — the contract this hook exists to uphold.
const stubFetch = (opts: { patchStatus?: number; patchBody?: unknown } = {}) => {
  const patches: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "PATCH") {
        patches.push(init.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse(opts.patchStatus ?? 200, opts.patchBody ?? { id: "ch-1" }));
      }
      return Promise.resolve(jsonResponse(200, { id: "ch-1" }));
    }),
  );
  return { patches };
};

const heat: LineupEntryDTO = { key: "movie:tmdb:949", name: "Heat", year: 1995 };
const pointBreak: LineupEntryDTO = { key: "movie:tmdb:9426", name: "Point Break", year: 1991 };
const predator: LineupEntryDTO = { key: "movie:tmdb:106", name: "Predator", year: 1987 };

afterEach(() => vi.restoreAllMocks());

describe("useChannelLineup", () => {
  it("exposes the current lineup unchanged before any commit", () => {
    stubFetch();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });
    expect(result.current.entries).toEqual([heat, pointBreak]);
    expect(result.current.isPending).toBe(false);
  });

  it("add appends the new entry and commits the full list, in order", async () => {
    const { patches } = stubFetch();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });

    act(() => {
      expect(result.current.add(predator)).toBe(true);
    });

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [heat, pointBreak, predator] });
  });

  it("a not-in-library (pending) add still commits — pending slots are a valid add", async () => {
    // The entry itself carries no "available" flag (LineupEntryDTO is {key,name,year,
    // genres} only) — pending-ness is a library-membership fact the hook has no reason
    // to know or gate on. Any well-formed entry commits the same way.
    const { patches } = stubFetch();
    const pending: LineupEntryDTO = { key: "series:tvdb:81189", name: "Breaking Bad", year: 2008 };
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat]), { wrapper: makeWrapper() });

    act(() => {
      expect(result.current.add(pending)).toBe(true);
    });

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [heat, pending] });
  });

  it("adding a duplicate key is prevented — no request is sent", () => {
    const { patches } = stubFetch();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });

    act(() => {
      expect(result.current.add({ key: heat.key, name: "Heat (dupe)", year: 1995 })).toBe(false);
    });

    expect(patches).toHaveLength(0);
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubFetch();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator]), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.remove(pointBreak.key));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [heat, predator] });
  });

  it("reorder commits arrayMove's result — the exact shape an onDragEnd hands it", async () => {
    const { patches } = stubFetch();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator]), {
      wrapper: makeWrapper(),
    });

    // Drag the last row (index 2) to the front (index 0).
    act(() => result.current.reorder(2, 0));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [predator, heat, pointBreak] });
  });

  it("surfaces isPending while a commit is in flight", async () => {
    let resolvePatch: (() => void) | undefined;
    const inFlight = new Promise<void>((resolve) => {
      resolvePatch = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === "PATCH") {
          return inFlight.then(() => jsonResponse(200, { id: "ch-1" }));
        }
        return Promise.resolve(jsonResponse(200, { id: "ch-1" }));
      }),
    );
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat]), { wrapper: makeWrapper() });

    act(() => {
      result.current.remove(heat.key);
    });

    await waitFor(() => expect(result.current.isPending).toBe(true));
    resolvePatch?.();
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });
});
