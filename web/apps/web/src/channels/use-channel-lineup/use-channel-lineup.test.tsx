import { channelsApi, type LineupEntryDTO } from "@loomarr/api";
import { getUpdateChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { useChannelLineup } from "./use-channel-lineup";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// Captures every PATCH body sent to /v1/channels/{id} so a test can assert exactly what
// the whole-list replace contained, in order — the contract this hook exists to uphold.
//
// ⚠ The stub this replaces dispatched on `init?.method === "PATCH"` ALONE, with no URL check at
// all — so it would have recorded a PATCH to any endpoint in the app as a lineup commit. It also
// answered every other request with `{ id: "ch-1" }`, which is ten fields short of a ChannelDTO.
const stubLineup = () => {
  const patches: unknown[] = [];
  server.use(
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel();
    }),
  );
  return { patches };
};

const heat: LineupEntryDTO = { key: "movie:tmdb:949", name: "Heat", year: 1995 };
const pointBreak: LineupEntryDTO = { key: "movie:tmdb:9426", name: "Point Break", year: 1991 };
const predator: LineupEntryDTO = { key: "movie:tmdb:106", name: "Predator", year: 1987 };

describe("useChannelLineup", () => {
  it("exposes the current lineup unchanged before any commit", () => {
    stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak], 1), {
      wrapper: makeWrapper(),
    });
    expect(result.current.entries).toEqual([heat, pointBreak]);
    expect(result.current.isPending).toBe(false);
  });

  it("add appends the new entry and commits the full list, in order", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak], 1), {
      wrapper: makeWrapper(),
    });

    act(() => {
      expect(result.current.add(predator)).toBe(true);
    });

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [heat, pointBreak, predator] });
  });

  it("a not-in-library (pending) add still commits — pending slots are a valid add", async () => {
    // The entry itself carries no "available" flag (LineupEntryDTO is {key,name,year,
    // genres} only) — pending-ness is a library-membership fact the hook has no reason
    // to know or gate on. Any well-formed entry commits the same way.
    const { patches } = stubLineup();
    const pending: LineupEntryDTO = { key: "series:tvdb:81189", name: "Breaking Bad", year: 2008 };
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat], 1), { wrapper: makeWrapper() });

    act(() => {
      expect(result.current.add(pending)).toBe(true);
    });

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [heat, pending] });
  });

  it("adding a duplicate key is prevented — no request is sent", () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak], 1), {
      wrapper: makeWrapper(),
    });

    act(() => {
      expect(result.current.add({ key: heat.key, name: "Heat (dupe)", year: 1995 })).toBe(false);
    });

    expect(patches).toHaveLength(0);
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator], 1), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.remove(pointBreak.key));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [heat, predator] });
  });

  it("reorder commits arrayMove's result — the exact shape an onDragEnd hands it", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator], 1), {
      wrapper: makeWrapper(),
    });

    // Drag the last row (index 2) to the front (index 0).
    act(() => result.current.reorder(2, 0));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [predator, heat, pointBreak] });
  });

  it("a stale 409 rolls back, refreshes the channel, and tells the operator", async () => {
    server.use(
      http.patch("*/v1/channels/:id", () =>
        HttpResponse.json(
          { title: "Channel changed", detail: "Refresh it, review the latest version, and try again." },
          { status: 409 },
        ),
      ),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak], 9), { wrapper });

    act(() => result.current.remove(pointBreak.key));
    expect(result.current.entries).toEqual([heat]);

    await waitFor(() => expect(result.current.entries).toEqual([heat, pointBreak]));
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: channelsApi.getGetChannelQueryKey("ch-1"),
    });
    expect(toast.error).toHaveBeenCalledWith("Channel changed");
  });

  it("surfaces isPending while a commit is in flight", async () => {
    // ⚠ The deferred promise is the point: `isPending` is only observable WHILE the request is
    // unresolved, so the resolver has to block. An MSW resolver may be async, so the await IS the
    // hold — no separate `.then()` on a stubbed fetch, and the route stays bound while it waits.
    let release: (() => void) | undefined;
    const inFlight = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      getUpdateChannelMockHandler(async () => {
        await inFlight;
        return channel();
      }),
    );
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat], 1), { wrapper: makeWrapper() });

    act(() => {
      result.current.remove(heat.key);
    });

    await waitFor(() => expect(result.current.isPending).toBe(true));
    release?.();
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });
});
