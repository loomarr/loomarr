import type { Proposal, ProposalDTO } from "@loomarr/api";
import { getGetProposalJobMockHandler, getRefineChannelMockHandler } from "@loomarr/api/msw";
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

// Two real endpoints: `POST /v1/channels/{id}/refine` starts the run and the authoritative
// `GET /v1/proposal-jobs/{jobId}` restores its lifecycle and optional Proposal.
//
// ⚠ The stub this replaced dispatched on METHOD ALONE — `if (init?.method === "POST")` — and its
// own comment said it did so "without pinning to exact URL strings". That was the honest
// description of a real weakness: any POST anywhere satisfied the refine branch, and every other
// request in the app fell into the proposals branch. Route-bound handlers remove the choice.
//
// The job response is externally resolvable so a test can observe the run's in-flight state
// before authoritative recovery lands the Proposal.
const stubRefine = () => {
  let resolveJob: ((rows: ProposalDTO[]) => void) | undefined;
  const jobRequested = new Promise<ProposalDTO[]>((resolve) => {
    resolveJob = resolve;
  });

  server.use(
    getRefineChannelMockHandler({ jobId: "job-1" }),
    getGetProposalJobMockHandler(async () => {
      const rows = await jobRequested;
      return {
        jobId: "job-1",
        status: rows.length > 0 ? "done" : "running",
        intent: proposal.intent,
        attempts: 1,
        createdAt: "2026-08-15T12:00:00Z",
        updatedAt: "2026-08-15T12:00:00Z",
        proposal: rows.find((candidate) => candidate.jobId === "job-1"),
      };
    }),
  );

  // Lets the job query settle with the given Proposal, once a test wants it to.
  return { landProposals: (rows: ProposalDTO[]) => resolveJob?.(rows) };
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

    // The job request is in flight (deliberately unresolved) — jobId is set, no
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

  it("retries a durable failure through Refine and tracks the fresh job", async () => {
    let starts = 0;
    server.use(
      getRefineChannelMockHandler(() => ({ jobId: starts++ === 0 ? "job-1" : "job-2" })),
      getGetProposalJobMockHandler(({ params }) => ({
        jobId: String(params.jobId),
        status: params.jobId === "job-1" ? "failed" : "running",
        intent: { ...proposal.intent, refineText: "add more action" },
        attempts: 1,
        createdAt: "2026-08-15T12:00:00Z",
        updatedAt: "2026-08-15T12:00:00Z",
        failure:
          params.jobId === "job-1" ? { code: "generation_failed", message: "Refine failed." } : undefined,
      })),
    );
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "add more action"));
    await waitFor(() => expect(result.current.failure).toBeDefined());
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.jobId).toBe("job-2"));
    expect(starts).toBe(2);
  });
});
