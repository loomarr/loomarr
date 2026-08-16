import type { Proposal, ProposalJobDTO } from "@loomarr/api";
import { getGetProposalJobMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useProposalJobTracker } from "./use-proposal-job-tracker";

const proposal: Proposal = {
  intent: { description: "90s action", era: "1990s" },
  lineup: [{ name: "Heat", mediaType: "movie", inLibrary: true }],
  acquisitions: [],
  alternates: [],
  scores: { themeFit: 1, availabilityRatio: 1, eraBalance: 1, overall: 1 },
};

const job = (status: ProposalJobDTO["status"], over: Partial<ProposalJobDTO> = {}): ProposalJobDTO => ({
  jobId: "job-1",
  status,
  intent: { description: "90s action", era: "1990s" },
  attempts: 1,
  createdAt: "2026-08-15T12:00:00Z",
  updatedAt: "2026-08-15T12:00:00Z",
  ...over,
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("useProposalJobTracker", () => {
  it("restores an existing job and polls until the authoritative GET returns done", async () => {
    let reads = 0;
    server.use(
      getGetProposalJobMockHandler(() => {
        reads++;
        return reads === 1
          ? job("queued")
          : job("done", { proposal: { id: "p-1", jobId: "job-1", status: "submitted", proposal } });
      }),
    );

    const { result } = renderHook(() => useProposalJobTracker({ jobId: "job-1" }), { wrapper });

    await waitFor(() => expect(result.current.job?.status).toBe("queued"));
    expect(result.current.isRunning).toBe(true);
    await waitFor(() => expect(result.current.proposal?.id).toBe("p-1"), { timeout: 2_500 });
    expect(reads).toBeGreaterThanOrEqual(2);
    expect(result.current.isRunning).toBe(false);
    expect(result.current.intent).toEqual({ description: "90s action", era: "1990s" });
  });

  it("preserves the authoritative safe failure and full intent", async () => {
    server.use(
      getGetProposalJobMockHandler(
        job("failed", {
          failure: { code: "no_grounded_titles", message: "No grounded titles matched." },
        }),
      ),
    );
    const { result } = renderHook(() => useProposalJobTracker({ jobId: "job-1" }), { wrapper });

    await waitFor(() => expect(result.current.failure?.code).toBe("no_grounded_titles"));
    expect(result.current.intent?.era).toBe("1990s");
    expect(result.current.isRunning).toBe(false);
  });

  it("keeps polling a submitted Proposal until another browser's decision is authoritative", async () => {
    let reads = 0;
    server.use(
      getGetProposalJobMockHandler(() => {
        reads++;
        return job("done", {
          proposal: {
            id: "p-1",
            jobId: "job-1",
            status: reads === 1 ? "submitted" : "approved",
            proposal,
          },
        });
      }),
    );

    const { result } = renderHook(() => useProposalJobTracker({ jobId: "job-1" }), { wrapper });

    await waitFor(() => expect(result.current.proposal?.status).toBe("submitted"));
    expect(result.current.isRunning).toBe(false);
    await waitFor(() => expect(result.current.proposal?.status).toBe("approved"), { timeout: 2_500 });
    expect(reads).toBeGreaterThanOrEqual(2);
  });
});
