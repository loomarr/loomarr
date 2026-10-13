import type { LineupEntryDTO } from "@loomarr/api";
import { getGetChannelMockHandler, getUpdateChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { delay, HttpResponse, http } from "msw";
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
// the whole-list replace contained, in order — the contract this hook exists to uphold. The
// channel writer reads the channel's revision first, so the by-id read answers too.
//
// ⚠ The stub this replaces dispatched on `init?.method === "PATCH"` ALONE, with no URL check at
// all — so it would have recorded a PATCH to any endpoint in the app as a lineup commit. It also
// answered every other request with `{ id: "ch-1" }`, which is ten fields short of a ChannelDTO.
const stubLineup = () => {
  const patches: unknown[] = [];
  server.use(
    getGetChannelMockHandler(channel({ revision: 1 })),
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel({ revision: 2 });
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
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });
    expect(result.current.entries).toEqual([heat, pointBreak]);
  });

  it("add appends the new entry and commits the full list, in order", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
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
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat]), { wrapper: makeWrapper() });

    act(() => {
      expect(result.current.add(pending)).toBe(true);
    });

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [heat, pending] });
  });

  it("adding a duplicate key is prevented — no request is sent", () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });

    act(() => {
      expect(result.current.add({ key: heat.key, name: "Heat (dupe)", year: 1995 })).toBe(false);
    });

    expect(patches).toHaveLength(0);
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator]), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.remove(pointBreak.key));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [heat, predator] });
  });

  it("reorder commits arrayMove's result — the exact shape an onDragEnd hands it", async () => {
    const { patches } = stubLineup();
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator]), {
      wrapper: makeWrapper(),
    });

    // Drag the last row (index 2) to the front (index 0).
    act(() => result.current.reorder(2, 0));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [predator, heat, pointBreak] });
  });

  it("a second op inside the round trip builds on the first and waits its turn (#1899)", async () => {
    const patches: { revision: number; lineup: LineupEntryDTO[] }[] = [];
    let revision = 1;
    server.use(
      getGetChannelMockHandler(channel({ revision: 1 })),
      http.patch("*/v1/channels/:id", async ({ request }) => {
        const body = (await request.json()) as { revision: number; lineup: LineupEntryDTO[] };
        patches.push(body);
        await delay(30);
        if (body.revision !== revision)
          return HttpResponse.json({ title: "Channel changed" }, { status: 409 });
        revision += 1;
        return HttpResponse.json(channel({ revision, lineup: body.lineup }));
      }),
    );
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak, predator]), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.remove(pointBreak.key));
    // Shown at once, so the next remove starts from it rather than the stale read.
    expect(result.current.entries).toEqual([heat, predator]);
    act(() => result.current.remove(predator.key));

    await waitFor(() => expect(patches).toHaveLength(2));
    expect(patches).toEqual([
      { revision: 1, lineup: [heat, predator] },
      { revision: 2, lineup: [heat] },
    ]);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("a failed save falls back to the saved lineup and tells the operator", async () => {
    server.use(
      getGetChannelMockHandler(channel({ revision: 1 })),
      http.patch("*/v1/channels/:id", () =>
        HttpResponse.json({ title: "Invalid lineup", detail: "That title can't be added." }, { status: 422 }),
      ),
    );
    const { result } = renderHook(() => useChannelLineup("ch-1", [heat, pointBreak]), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.remove(pointBreak.key));
    expect(result.current.entries).toEqual([heat]);

    await waitFor(() => expect(result.current.entries).toEqual([heat, pointBreak]));
    expect(toast.error).toHaveBeenCalledWith("Invalid lineup");
  });
});
