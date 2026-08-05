import type { Proposal } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useChannelRefine } from "./use-channel-refine";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const proposal: Proposal = {
  intent: { description: "90s action movies" },
  lineup: [{ name: "Heat", year: 1995, mediaType: "movie", inLibrary: true }],
  acquisitions: [],
  alternates: [],
  scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
};

// Dispatches by method: POST is the refine trigger, GET is the approval-queue poll —
// mirroring the two real endpoints (`POST /v1/channels/{id}/refine`, `GET
// /v1/proposals?status=submitted`) without pinning to exact URL strings. The
// proposals response is deferred (an externally-resolvable promise) rather than
// resolved immediately, so a test can observe the run's "in flight" state before
// choosing to let the list respond — this hook's list query has no refetchInterval
// (same as useSuggestionRun, which it mirrors: neither polls on a timer), so it only
// ever resolves once per enable, and there is nothing to race by resolving it eagerly.
const stubFetch = (opts: { refineStatus?: number; refineBody?: unknown } = {}) => {
  let resolveProposals: ((rows: unknown[]) => void) | undefined;
  const proposalsRequested = new Promise<unknown[]>((resolve) => {
    resolveProposals = resolve;
  });

  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve(jsonResponse(opts.refineStatus ?? 200, opts.refineBody ?? { jobId: "job-1" }));
      }
      return proposalsRequested.then((rows) => jsonResponse(200, { proposals: rows }));
    }),
  );

  // Lets the list query settle with the given rows, once a test wants it to.
  return { landProposals: (rows: unknown[]) => resolveProposals?.(rows) };
};

afterEach(() => vi.restoreAllMocks());

describe("useChannelRefine", () => {
  it("starts idle with no phase or proposal", () => {
    stubFetch();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });
    expect(result.current.isRunning).toBe(false);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.phase).toBeUndefined();
  });

  it("starts a run on submit and flips isRunning until the proposal lands", async () => {
    const { landProposals } = stubFetch();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "add more Schwarzenegger"));

    // The list request is in flight (deliberately unresolved) — jobId is set, no
    // proposal yet, so the run reads as running.
    await waitFor(() => expect(result.current.isRunning).toBe(true));
    expect(result.current.proposal).toBeUndefined();

    act(() => landProposals([{ id: "p1", jobId: "job-1", status: "submitted", proposal }]));

    await waitFor(() => expect(result.current.proposal?.jobId).toBe("job-1"));
    expect(result.current.isRunning).toBe(false);
  });

  it("only matches the proposal whose jobId equals the started run's", async () => {
    const { landProposals } = stubFetch();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "less horror, more action"));
    act(() =>
      landProposals([
        { id: "other", jobId: "job-other", status: "submitted", proposal },
        { id: "mine", jobId: "job-1", status: "submitted", proposal },
      ]),
    );

    await waitFor(() => expect(result.current.proposal?.id).toBe("mine"));
  });

  it("resets back to idle", async () => {
    const { landProposals } = stubFetch();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "swap the finale"));
    act(() => landProposals([{ id: "p1", jobId: "job-1", status: "submitted", proposal }]));
    await waitFor(() => expect(result.current.proposal).toBeDefined());

    act(() => result.current.reset());
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.phase).toBeUndefined();
    expect(result.current.isRunning).toBe(false);
  });

  it("surfaces the mutation error when the refine call fails", async () => {
    stubFetch({ refineStatus: 422, refineBody: { title: "Change is required" } });
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", ""));

    await waitFor(() => expect(result.current.error).toBeDefined());
  });
});
