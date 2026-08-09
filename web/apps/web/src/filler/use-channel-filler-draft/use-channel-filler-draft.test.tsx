import type { ChannelPolicy, PodPoolDTO } from "@loomarr/api";
import { getPreviewDraftChannelPodsMockHandler, getUpdateChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelFillerDraft } from "./use-channel-filler-draft";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const previewBody: PodPoolDTO = {
  entries: [
    {
      path: "p1.mp4",
      tunarrProgramId: "p1",
      name: "Ad",
      kind: "commercial",
      durationMs: 30000,
      isFallbackCard: false,
    },
  ],
  totalMs: 30000,
  matchLevel: "exact",
};

// Records what the preview POST / apply PATCH sent, each from the resolver bound to its own route:
// POST …/pods/preview is the draft preview, PATCH …/channels/{id} is apply.
//
// ⚠ `previewStatus` stays HAND-WRITTEN, and it has to be: the spec declares errors via `default:`
// (RFC 7807) with no explicit 422, so orval generates nothing to fail with. The generated handler
// is skipped entirely in that mode rather than layered under a guard — there is only one request
// in flight and it is the one that must fail.
const stubDraft = (opts: { previewStatus?: number } = {}) => {
  const previews: unknown[] = [];
  const saves: unknown[] = [];

  server.use(
    opts.previewStatus
      ? http.post("*/v1/channels/:id/pods/preview", () =>
          HttpResponse.json({ title: "Unprocessable" }, { status: opts.previewStatus }),
        )
      : getPreviewDraftChannelPodsMockHandler(async ({ request }) => {
          previews.push(await request.json());
          return previewBody;
        }),
    getUpdateChannelMockHandler(async ({ request }) => {
      saves.push(await request.json());
      return channel();
    }),
  );

  return { previews, saves };
};

const policy = (filler?: ChannelPolicy["filler"]): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  ...(filler ? { filler } : {}),
});

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("canonicalize", () => {
  it("folds empty-means-any: [] and undefined and 0-field era all read identical", () => {
    expect(canonicalize({})).toBe(canonicalize({ categories: [], kinds: [], pinned: [], excluded: [] }));
    expect(canonicalize({})).toBe(canonicalize({ audience: "", era: {} }));
  });
  it("is order-insensitive within a list (membership, not order, is identity)", () => {
    expect(canonicalize({ pinned: ["a", "b"] })).toBe(canonicalize({ pinned: ["b", "a"] }));
  });
  it("distinguishes a real difference", () => {
    expect(canonicalize({ audience: "kids" })).not.toBe(canonicalize({ audience: "family" }));
  });
});

describe("useChannelFillerDraft", () => {
  it("seeds the draft from policy.filler", () => {
    stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    expect(result.current.draft.audience).toBe("kids");
    expect(result.current.isDirty).toBe(false);
  });

  it("fires a debounced preview POST with the canonical draft, and renders its result", async () => {
    const { previews } = stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });

    // Edit the draft; the POST must NOT fire until the debounce elapses.
    act(() => result.current.setDraft({ audience: "kids", categories: ["toys"] }));
    expect(previews).toHaveLength(0);

    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });

    await waitFor(() => expect(result.current.preview?.entries).toHaveLength(1));
    expect(previews.at(-1)).toMatchObject({ filler: { audience: "kids", categories: ["toys"] } });
  });

  it("coalesces a burst of edits into a single preview POST", async () => {
    const { previews } = stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });

    act(() => result.current.setDraft({ audience: "kids" }));
    act(() => result.current.setDraft({ audience: "family" }));
    act(() => result.current.setDraft({ audience: "general" }));
    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });

    // The initial mount preview (empty draft) plus exactly one for the settled burst.
    await waitFor(() => expect(previews.at(-1)).toMatchObject({ filler: { audience: "general" } }));
    // Three rapid edits collapsed — not three separate POSTs for them.
    expect(previews.filter((p) => (p as { filler: { audience?: string } }).filler.audience).length).toBe(1);
  });

  it("flips isDirty when the draft diverges and back when it matches saved", () => {
    stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    expect(result.current.isDirty).toBe(false);
    act(() => result.current.setDraft({ audience: "family" }));
    expect(result.current.isDirty).toBe(true);
    act(() => result.current.setDraft({ audience: "kids" }));
    expect(result.current.isDirty).toBe(false);
  });

  it("apply PATCHes the draft MERGED onto the rest of the saved policy", async () => {
    const { saves } = stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    act(() => result.current.setDraft({ audience: "family", pinned: ["p9"] }));
    act(() => result.current.apply());

    // ⚠ Was `calls.some(c => c.method === "PATCH")`, satisfied by a PATCH to anything.
    await waitFor(() => expect(saves).toHaveLength(1));
    // The filler is the new draft…
    expect(saves[0]).toMatchObject({ policy: { filler: { audience: "family", pinned: ["p9"] } } });
    // …and the rest of the policy is carried, not wiped (PATCH replaces policy whole).
    expect(saves[0]).toMatchObject({ policy: { ordering: "shuffle", scope: { era: { from: 1990 } } } });
  });

  it("discard resets the draft to saved", () => {
    stubDraft();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    act(() => result.current.setDraft({ audience: "late_night" }));
    expect(result.current.isDirty).toBe(true);
    act(() => result.current.discard());
    expect(result.current.draft.audience).toBe("kids");
    expect(result.current.isDirty).toBe(false);
  });

  it("surfaces a preview failure instead of an empty timeline", async () => {
    stubDraft({ previewStatus: 422 });
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });
    act(() => result.current.setDraft({ audience: "kids" }));
    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });
    await waitFor(() => expect(result.current.previewError).toBeDefined());
  });
});
