import type { Proposal, ProposalJourneyDTO } from "@loomarr/api";
import { getGetProposalJobMockHandler, getRefineChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
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

const journey = (over: Partial<ProposalJourneyDTO> = {}): ProposalJourneyDTO => ({
  version: 1,
  jobId: "job-1",
  milestone: "generating",
  intent: proposal.intent,
  attempts: [],
  actions: ["wait"],
  createdAt: "2026-08-22T10:00:00Z",
  updatedAt: "2026-08-22T10:00:00Z",
  ...over,
});

// The Journey response is externally resolvable so a test can observe the run
// in flight before its authoritative state lands.
const stubRefine = () => {
  let resolveJourney: ((value: ProposalJourneyDTO) => void) | undefined;
  const journeyRequested = new Promise<ProposalJourneyDTO>((resolve) => {
    resolveJourney = resolve;
  });

  server.use(
    getRefineChannelMockHandler({ jobId: "job-1" }),
    getGetProposalJobMockHandler(async () => journeyRequested),
  );

  return { landJourney: (value: ProposalJourneyDTO) => resolveJourney?.(value) };
};

// ⚠ Hand-written: the failure is a STATUS (422), and this spec declares errors with `default:`
// (RFC 7807) rather than enumerating 4xx, so orval has no code to generate a handler from. A
// rename still fails loudly — the stale path stops matching and the real request goes unhandled.
const refineRejects = () =>
  http.post("*/v1/channels/:id/refine", () =>
    HttpResponse.json({ title: "Change is required" }, { status: 422 }),
  );

afterEach(() => window.sessionStorage.clear());

describe("useChannelRefine", () => {
  it("starts idle with no phase or proposal", () => {
    stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });
    expect(result.current.isRunning).toBe(false);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.phase).toBeUndefined();
  });

  it("starts a run on submit and flips isRunning until the proposal lands", async () => {
    const { landJourney } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "add more Schwarzenegger"));

    // The list request is in flight (deliberately unresolved) — jobId is set, no
    // proposal yet, so the run reads as running.
    await waitFor(() => expect(result.current.isRunning).toBe(true));
    expect(result.current.proposal).toBeUndefined();

    act(() =>
      landJourney(
        journey({ milestone: "awaiting_approval", proposal: { id: "p1", status: "submitted", proposal } }),
      ),
    );

    await waitFor(() => expect(result.current.proposal?.jobId).toBe("job-1"));
    expect(result.current.isRunning).toBe(false);
  });

  it("restores a started run from session storage after reload", async () => {
    const { landJourney } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "less horror, more action"));
    await waitFor(() => expect(result.current.isRunning).toBe(true));
    act(() =>
      landJourney(
        journey({ milestone: "awaiting_approval", proposal: { id: "mine", status: "submitted", proposal } }),
      ),
    );
    await waitFor(() => expect(result.current.proposal?.id).toBe("mine"));

    const restored = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });
    await waitFor(() => expect(restored.result.current.proposal?.id).toBe("mine"));
    expect(restored.result.current.channelId).toBe("ch-1");
  });

  it("resets back to idle", async () => {
    const { landJourney } = stubRefine();
    const { result } = renderHook(() => useChannelRefine(), { wrapper: makeWrapper() });

    act(() => result.current.start("ch-1", "swap the finale"));
    act(() =>
      landJourney(
        journey({ milestone: "awaiting_approval", proposal: { id: "p1", status: "submitted", proposal } }),
      ),
    );
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
