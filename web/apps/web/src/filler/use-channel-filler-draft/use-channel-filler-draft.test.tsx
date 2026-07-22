import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelFillerDraft } from "./use-channel-filler-draft";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const previewBody = {
  entries: [
    { tunarrProgramId: "p1", name: "Ad", kind: "commercial", durationMs: 30000, isFallbackCard: false },
  ],
  totalMs: 30000,
  matchLevel: "exact",
};

// Records every request so a test can assert what the preview POST / apply PATCH sent, and
// dispatches a canned response by method + path: POST …/pods/preview is the draft preview,
// PATCH …/channels/{id} is apply. Bodies are captured parsed.
const stubFetch = (opts: { previewStatus?: number } = {}) => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(init.body as string) : undefined;
      calls.push({ method, url: String(url), body });
      if (method === "POST" && String(url).endsWith("/pods/preview")) {
        return Promise.resolve(jsonResponse(opts.previewStatus ?? 200, previewBody));
      }
      if (method === "PATCH") return Promise.resolve(jsonResponse(200, { id: "ch-1" }));
      // The catalog read (GET /v1/filler) the hook's callers use — harmless empty here.
      return Promise.resolve(jsonResponse(200, { clips: [] }));
    }),
  );
  return { calls };
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
    stubFetch();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    expect(result.current.draft.audience).toBe("kids");
    expect(result.current.isDirty).toBe(false);
  });

  it("fires a debounced preview POST with the canonical draft, and renders its result", async () => {
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });

    // Edit the draft; the POST must NOT fire until the debounce elapses.
    act(() => result.current.setDraft({ audience: "kids", categories: ["toys"] }));
    expect(calls.filter((c) => c.url.endsWith("/pods/preview"))).toHaveLength(0);

    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });

    await waitFor(() => expect(result.current.preview?.entries).toHaveLength(1));
    const post = calls.filter((c) => c.url.endsWith("/pods/preview")).at(-1);
    expect(post?.body).toMatchObject({ filler: { audience: "kids", categories: ["toys"] } });
  });

  it("coalesces a burst of edits into a single preview POST", async () => {
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });

    act(() => result.current.setDraft({ audience: "kids" }));
    act(() => result.current.setDraft({ audience: "family" }));
    act(() => result.current.setDraft({ audience: "general" }));
    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });

    // The initial mount preview (empty draft) plus exactly one for the settled burst.
    const posts = calls.filter((c) => c.url.endsWith("/pods/preview"));
    await waitFor(() => expect(posts.at(-1)?.body).toMatchObject({ filler: { audience: "general" } }));
    // Three rapid edits collapsed — not three separate POSTs for them.
    expect(posts.filter((p) => (p.body as { filler: { audience?: string } }).filler.audience).length).toBe(1);
  });

  it("flips isDirty when the draft diverges and back when it matches saved", () => {
    stubFetch();
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
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy({ audience: "kids" })), {
      wrapper: makeWrapper(),
    });
    act(() => result.current.setDraft({ audience: "family", pinned: ["p9"] }));
    act(() => result.current.apply());

    await waitFor(() => expect(calls.some((c) => c.method === "PATCH")).toBe(true));
    const patch = calls.find((c) => c.method === "PATCH");
    // The filler is the new draft…
    expect(patch?.body).toMatchObject({ policy: { filler: { audience: "family", pinned: ["p9"] } } });
    // …and the rest of the policy is carried, not wiped (PATCH replaces policy whole).
    expect(patch?.body).toMatchObject({ policy: { ordering: "shuffle", scope: { era: { from: 1990 } } } });
  });

  it("discard resets the draft to saved", () => {
    stubFetch();
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
    stubFetch({ previewStatus: 422 });
    const { result } = renderHook(() => useChannelFillerDraft("ch-1", policy()), { wrapper: makeWrapper() });
    act(() => result.current.setDraft({ audience: "kids" }));
    await act(async () => {
      vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS);
    });
    await waitFor(() => expect(result.current.previewError).toBeDefined());
  });
});
