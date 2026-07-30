import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelRulesDraft } from "./use-channel-rules-draft";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const previewBody = {
  at: "2026-07-30T12:00:00Z",
  activeRule: { id: "r1", label: "Weekend marathon", priority: 5, matched: true },
  windowMs: 0,
  slots: [],
  pods: { entries: [], totalMs: 0, matchLevel: "exact" },
};

// Records every request so a test can assert what the preview POST / apply PATCH actually
// SENT — the wire shape, not a mocked hook's arguments.
const stubFetch = (opts: { previewStatus?: number } = {}) => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(init.body as string) : undefined;
      calls.push({ method, url: String(url), body });
      if (method === "POST" && String(url).includes("/programming/preview")) {
        return Promise.resolve(jsonResponse(opts.previewStatus ?? 200, previewBody));
      }
      if (method === "PATCH") return Promise.resolve(jsonResponse(200, { id: "ch-1" }));
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
  return { calls };
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
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy()), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.setDraft(policy({ ordering: "sequential" })));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));

    await waitFor(() => {
      const post = calls.find((c) => c.method === "POST" && c.url.includes("/programming/preview"));
      expect(post).toBeDefined();
      // The EDITED policy is on the wire — the assertion that this previews the draft rather
      // than re-reading what is saved.
      expect((post?.body as { policy: ChannelPolicy }).policy.ordering).toBe("sequential");
    });
  });

  // ⚠ The whole reason this surface drafts: rules are interdependent, so intermediate states
  // must not reach Tunarr. Nothing may PATCH until apply is called.
  it("does NOT save while editing — only apply writes", async () => {
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy()), {
      wrapper: makeWrapper(),
    });

    act(() => result.current.setDraft(policy({ ordering: "sequential" })));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));
    await waitFor(() => expect(calls.some((c) => c.method === "POST")).toBe(true));

    expect(calls.some((c) => c.method === "PATCH")).toBe(false);

    act(() => result.current.apply());
    await waitFor(() => {
      const patch = calls.find((c) => c.method === "PATCH");
      expect(patch).toBeDefined();
      expect((patch?.body as { policy: ChannelPolicy }).policy.ordering).toBe("sequential");
    });
  });

  it("is dirty only once the draft differs, and discard returns to saved", () => {
    stubFetch();
    const saved = policy();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", saved), {
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
    stubFetch();
    const { result, rerender } = renderHook(({ p }) => useChannelRulesDraft("ch-1", p), {
      wrapper: makeWrapper(),
      initialProps: { p: policy() },
    });

    rerender({ p: policy() }); // same content, different reference
    expect(result.current.isDirty).toBe(false);
  });

  // A genuine server update (an apply landing, a refine reseeding rules) must ADOPT — otherwise
  // the draft would silently shadow what the server now holds.
  it("adopts a genuinely changed saved policy", () => {
    stubFetch();
    const { result, rerender } = renderHook(({ p }) => useChannelRulesDraft("ch-1", p), {
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
    const { calls } = stubFetch();
    const { result } = renderHook(() => useChannelRulesDraft("ch-1", policy()), {
      wrapper: makeWrapper(),
    });

    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));
    await waitFor(() => expect(calls.filter((c) => c.method === "POST").length).toBe(1));

    act(() => result.current.setAt("2026-12-25T09:00:00Z"));
    act(() => void vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS + 10));

    await waitFor(() => {
      const posts = calls.filter((c) => c.method === "POST");
      expect(posts.length).toBe(2);
      expect(posts[1]?.url).toContain("at=");
    });
  });
});
