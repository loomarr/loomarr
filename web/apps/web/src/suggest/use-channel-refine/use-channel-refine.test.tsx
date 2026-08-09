import type { Proposal, ProposalDTO } from "@loomarr/api";
import { getListProposalsMockHandler, getRefineChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useChannelRefine } from "./use-channel-refine";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const proposal: Proposal = {
  intent: { description: "90s action movies" },
  lineup: [{ name: "Heat", year: 1995, mediaType: "movie", inLibrary: true }],
  acquisitions: [],
  alternates: [],
  scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
};

// Two real endpoints: `POST /v1/channels/{id}/refine` starts the run, `GET /v1/proposals` is the
// approval-queue read the hook watches for its jobId.
//
// ⚠ The stub this replaced dispatched on METHOD ALONE — `if (init?.method === "POST")` — and its
// own comment said it did so "without pinning to exact URL strings". That was the honest
// description of a real weakness: any POST anywhere satisfied the refine branch, and every other
// request in the app fell into the proposals branch. Route-bound handlers remove the choice.
//
// ⚠ THE DEFERRAL IS PRESERVED, and it is load-bearing rather than incidental. The list response is
// an externally-resolvable promise so a test can observe the run's "in flight" state before letting
// the list answer. MSW supports this directly — a resolver may return a promise — so the mechanism
// survives the migration intact. This hook's list query has no refetchInterval (same as
// useSuggestionRun, which it mirrors: neither polls on a timer), so it resolves once per enable and
// there is nothing to race by resolving eagerly.
const stubRefine = () => {
  let resolveProposals: ((rows: ProposalDTO[]) => void) | undefined;
  const proposalsRequested = new Promise<ProposalDTO[]>((resolve) => {
    resolveProposals = resolve;
  });

  server.use(
    getRefineChannelMockHandler({ jobId: "job-1" }),
    getListProposalsMockHandler(async () => ({ proposals: await proposalsRequested })),
  );

  // Lets the list query settle with the given rows, once a test wants it to.
  return { landProposals: (rows: ProposalDTO[]) => resolveProposals?.(rows) };
};

// ⚠ Hand-written: the failure is a STATUS (422), and this spec declares errors with `default:`
// (RFC 7807) rather than enumerating 4xx, so orval has no code to generate a handler from. A
// rename still fails loudly — the stale path stops matching and the real request goes unhandled.
const refineRejects = () =>
  http.post("*/v1/channels/:id/refine", () =>
    HttpResponse.json({ title: "Change is required" }, { status: 422 }),
  );

const row = (over: Partial<ProposalDTO> & Pick<ProposalDTO, "id" | "jobId">): ProposalDTO => ({
  status: "submitted",
  proposal,
  ...over,
});

describe("useChannelRefine", () => {
  it("starts idle with no phase or proposal", () => {
    stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });
    expect(result.current.isRunning).toBe(false);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.phase).toBeUndefined();
  });

  it("starts a run on submit and flips isRunning until the proposal lands", async () => {
    const { landProposals } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "add more Schwarzenegger"));

    // The list request is in flight (deliberately unresolved) — jobId is set, no
    // proposal yet, so the run reads as running.
    await waitFor(() => expect(result.current.isRunning).toBe(true));
    expect(result.current.proposal).toBeUndefined();

    act(() => landProposals([row({ id: "p1", jobId: "job-1" })]));

    await waitFor(() => expect(result.current.proposal?.jobId).toBe("job-1"));
    expect(result.current.isRunning).toBe(false);
  });

  it("only matches the proposal whose jobId equals the started run's", async () => {
    const { landProposals } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "less horror, more action"));
    act(() => landProposals([row({ id: "other", jobId: "job-other" }), row({ id: "mine", jobId: "job-1" })]));

    await waitFor(() => expect(result.current.proposal?.id).toBe("mine"));
  });

  it("resets back to idle", async () => {
    const { landProposals } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "swap the finale"));
    act(() => landProposals([row({ id: "p1", jobId: "job-1" })]));
    await waitFor(() => expect(result.current.proposal).toBeDefined());

    act(() => result.current.reset());
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.phase).toBeUndefined();
    expect(result.current.isRunning).toBe(false);
  });

  it("surfaces the mutation error when the refine call fails", async () => {
    server.use(refineRejects());
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", ""));

    await waitFor(() => expect(result.current.error).toBeDefined());
  });
});
