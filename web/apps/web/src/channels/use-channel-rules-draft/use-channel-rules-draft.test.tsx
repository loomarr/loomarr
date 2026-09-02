import type { ChannelPolicy, PreviewProgrammingOutputBody } from "@loomarr/api";
import { getPreviewChannelProgrammingMockHandler, getUpdateChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelRulesDraft } from "./use-channel-rules-draft";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const previewBody: PreviewProgrammingOutputBody = {
  at: "2026-07-30T12:00:00Z",
  activeRule: { id: "r1", label: "Weekend marathon", priority: 5, matched: true },
  windowMs: 0,
  slots: [],
  pods: { entries: [], totalMs: 0, matchLevel: "exact" },
  // Empty-but-present, like every other required array in this fixture: this hook's tests are
  // about the draft round-trip, not about what the §4 filters refused.
  excluded: { overCeiling: 0, unrated: 0, items: [] },
  trace: {
    version: 1,
    ordering: "shuffle",
    seed: "0",
    windowMs: 0,
    windowIndex: 0,
    relaxations: [],
    factTotal: 0,
    recordedTotal: 0,
    truncated: false,
    facts: [],
  },
};

// Records what the preview POST / apply PATCH actually SENT — the wire shape, not a mocked
// hook's arguments.
//
// ⚠ Two separate lists, because they are two separate ROUTES. The stub this replaces kept one
// `calls` array and filtered it by `c.method === "PATCH"` — true of a PATCH to anything — and by
// `c.url.includes("/programming/preview")`, a string the test itself supplied. Splitting them at
// the resolver means "a preview happened" and "a save happened" can no longer be confused, which
// matters here more than usual: the central claim of this hook is that editing previews and does
// NOT save, so the two must be distinguishable by construction.
const stubDraft = () => {
  const previews: { url: string; body: unknown }[] = [];
  const saves: { body: unknown }[] = [];
  server.use(
    getPreviewChannelProgrammingMockHandler(async ({ request }) => {
      previews.push({ url: request.url, body: await request.json() });
      return previewBody;
    }),
    getUpdateChannelMockHandler(async ({ request }) => {
      saves.push({ body: await request.json() });
      return channel();
    }),
  );
  return { previews, saves };
};

const policy = (over: Partial<ChannelPolicy> = {}): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  ...over,
});

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("canonicalize", () => {
  // ⚠ Key ORDER must not read as an edit: a policy that round-tripped through the server can
  // come back with its keys in a different order, and a raw JSON.stringify would call that dirty.
  it("is key-order insensitive", () => {
    expect(canonicalize({ ordering: "shuffle", scope: {} })).toBe(
      canonicalize({ scope: {}, ordering: "shuffle" } as ChannelPolicy),
    );
  });

  // ⚠ Rule ORDER *is* identity — list order is priority (programming-design §6.6). Sorting
  // rules would make drag-to-reorder invisible to the dirty check, which is exactly the edit
  // this draft exists to let you preview before it ships.
  it("treats a rule REORDER as a real change", () => {
    const a = { rules: [{ id: "x" }, { id: "y" }] } as unknown as ChannelPolicy;
    const b = { rules: [{ id: "y" }, { id: "x" }] } as unknown as ChannelPolicy;
    expect(canonicalize(a)).not.toBe(canonicalize(b));
  });

  it("distinguishes a real difference", () => {
    expect(canonicalize({ ordering: "shuffle" })).not.toBe(canonicalize({ ordering: "sequential" }));
  });
});

describe("useChannelRulesDraft", () => {
  it("previews the DRAFT through POST …/programming/preview, not the saved policy", async () => {
    const { previews } = stubDraft();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy(), 1), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.setDraft(policy({ ordering: "sequential" })));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));

    await waitFor(() => {
      // The EDITED policy is on the wire — the assertion that this previews the draft rather
      // than re-reading what is saved. Reaching the resolver bound to
      // `POST /v1/channels/:id/programming/preview` is what proves the ROUTE.
      //
      // ⚠ Destructured rather than `(previews[0]?.body as T).policy` — Biome rightly refuses
      // that shape, since a short-circuit to `undefined` would THROW here instead of failing
      // the assertion.
      const [first] = previews;
      if (!first) throw new Error("no preview reached the route");
      expect((first.body as { policy: ChannelPolicy }).policy.ordering).toBe("sequential");
    });
  });

  // ⚠ The whole reason this surface drafts: rules are interdependent, so intermediate states
  // must not reach Tunarr. Nothing may PATCH until apply is called.
  it("does NOT save while editing — only apply writes", async () => {
    const { previews, saves } = stubDraft();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy(), 1), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.setDraft(policy({ ordering: "sequential" })));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));
    await waitFor(() => expect(previews).not.toHaveLength(0));

    // ⚠ Was `calls.some(c => c.method === "PATCH")` — which a PATCH to ANY endpoint would have
    // satisfied. `saves` can only be fed by `PATCH /v1/channels/:id`.
    expect(saves).toHaveLength(0);

    act(() => result.current.apply());
    await waitFor(() => {
      const [first] = saves;
      if (!first) throw new Error("apply did not reach PATCH /v1/channels/:id");
      expect((first.body as { policy: ChannelPolicy }).policy.ordering).toBe("sequential");
    });
  });

  it("is dirty only once the draft differs, and discard returns to saved", () => {
    stubDraft();
    const saved = policy();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", saved, 1), {
      wrapper: makeWrapper(),
    });

    expect(result.current.isDirty).toBe(false);

    act(() => result.current.setDraft(policy({ ordering: "sequential" })));
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.discard());
    expect(result.current.isDirty).toBe(false);
    expect(result.current.draft.ordering).toBe("shuffle");
  });

  // A fresh object with identical content arrives on every parent render. Treating that as an
  // edit would show an Apply bar nobody asked for, on a page nobody touched.
  it("a re-render with an equal-but-new policy object is not dirty", () => {
    stubDraft();
    const { result, rerender } = renderHook(({ p }) => useChannelRulesDraft("ch-1", p, 1), {
      wrapper: makeWrapper(),
      initialProps: { p: policy() },
    });

    rerender({ p: policy() }); // same content, different reference
    expect(result.current.isDirty).toBe(false);
  });

  // A genuine server update (an apply landing, a refine reseeding rules) must ADOPT — otherwise
  // the draft would silently shadow what the server now holds.
  it("adopts a genuinely changed saved policy", () => {
    stubDraft();
    const { result, rerender } = renderHook(({ p }) => useChannelRulesDraft("ch-1", p, 1), {
      wrapper: makeWrapper(),
      initialProps: { p: policy() },
    });

    rerender({ p: policy({ ordering: "sequential" }) });
    expect(result.current.draft.ordering).toBe("sequential");
    expect(result.current.isDirty).toBe(false);
  });

  // `at` is a preview INPUT, so changing it must re-POST — otherwise time-travel would silently
  // keep showing the previous moment.
  it("re-previews when the evaluation time changes", async () => {
    const { previews } = stubDraft();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy(), 1), {
      wrapper: makeWrapper(),
    });

    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));
    await waitFor(() => expect(previews).toHaveLength(1));

    act(() => result.current.setAt("2026-12-25T09:00:00Z"));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));

    await waitFor(() => {
      expect(previews).toHaveLength(2);
      // The url is read off the REQUEST the resolver received, so `at=` is being asserted on
      // what the hook actually sent rather than on a string the stub echoed back.
      expect(previews[1]?.url).toContain("at=");
    });
  });
});
