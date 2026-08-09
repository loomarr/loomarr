import type { Proposal, ProposalDTO } from "@loomarr/api";
import {
  getApproveProposalMockHandler,
  getListProposalsMockHandler,
  getRefineChannelMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrEventsProvider } from "@/events";
import { server } from "@/test/msw/server";
import { RefinePanel } from "./refine-panel";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// A minimal EventSource stand-in that lets a test push SSE frames. jsdom has no
// EventSource, and the failed-run path depends on a real `suggestion` frame reaching the
// panel (via LoomarrEventsProvider → useChannelRefine.onSuggestion) — a no-op stub can't
// exercise it. `emit(type, data)` dispatches to the matching addEventListener callback.
class MockEventSource {
  static last: MockEventSource | undefined;
  private handlers = new Map<string, (e: MessageEvent) => void>();
  constructor() {
    MockEventSource.last = this;
  }
  addEventListener(type: string, cb: (e: MessageEvent) => void) {
    this.handlers.set(type, cb);
  }
  close() {}
  emit(type: string, data: unknown) {
    this.handlers.get(type)?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

// Wraps in the events provider so onSuggestion frames actually fan out (the plain
// makeWrapper omits it, which is fine for the happy path where the enable-fetch alone
// lands the proposal).
const makeEventfulWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
    </QueryClientProvider>
  );
};

const proposal: Proposal = {
  intent: { description: "add more Schwarzenegger" },
  lineup: [{ name: "Predator", year: 1987, mediaType: "movie", tmdbId: 106, inLibrary: true }],
  acquisitions: [],
  alternates: [],
  scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
};

// Dispatches by method+path: POST .../refine starts the run, GET /v1/proposals is the
// approval-queue poll that lands the proposal, POST .../approve applies it.
// ⚠ Three route-bound handlers replacing three substring branches, and the last one was the
// dangerous shape: an unconditional `{ proposals }` for EVERY unmatched request, at any path.
// `url.includes("/approve")` was also true of `POST /v1/proposals/approve` (the BULK route), which
// is a different endpoint from the per-proposal one this panel calls.
const stubRefine = (opts: { proposals: ProposalDTO[] }) =>
  server.use(
    getRefineChannelMockHandler({ jobId: "job-1" }),
    // ⚠ `status` is REQUIRED on ApproveOutputBody and no stub ever sent it.
    getApproveProposalMockHandler({ channelId: "ch-1", enqueued: 0, status: "approved" }),
    getListProposalsMockHandler({ proposals: opts.proposals }),
  );

// ⚠ `unstubAllGlobals`, not `restoreAllMocks`: the last test installs MockEventSource with
// `vi.stubGlobal`, and restoreAllMocks does not undo a stubbed global — it would leak into the
// next file's EventSource.
afterEach(() => vi.unstubAllGlobals());

describe("RefinePanel", () => {
  it("starts collapsed with the entry point only", () => {
    stubRefine({ proposals: [] });
    render(<RefinePanel channelId="ch-1" channelName="90s Action" />, { wrapper: makeWrapper() });
    expect(screen.getByRole("button", { name: /refine with ai/i })).toBeInTheDocument();
    expect(screen.queryByLabelText("What to change")).not.toBeInTheDocument();
  });

  it("opens the textarea on click and disables Refine until there's text", async () => {
    stubRefine({ proposals: [] });
    render(<RefinePanel channelId="ch-1" channelName="90s Action" />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /refine with ai/i }));
    expect(screen.getByLabelText("What to change")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^refine$/i })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("What to change"), "add more action");
    expect(screen.getByRole("button", { name: /^refine$/i })).toBeEnabled();
  });

  it("runs the full idle -> running -> landed flow and applies the diff", async () => {
    stubRefine({
      proposals: [{ id: "p1", jobId: "job-1", status: "submitted", proposal }],
    });
    const onApplied = vi.fn();
    render(<RefinePanel channelId="ch-1" channelName="90s Action" onApplied={onApplied} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(screen.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(screen.getByRole("button", { name: /^refine$/i }));

    // Landed: the diff shows the new pick, with Apply available.
    const applyButton = await screen.findByRole("button", { name: /apply changes/i });
    expect(screen.getByText("Predator")).toBeInTheDocument();

    await userEvent.click(applyButton);

    await waitFor(() => expect(onApplied).toHaveBeenCalledOnce());
    // Applying closes the panel back to its idle entry point.
    expect(screen.queryByText("Predator")).not.toBeInTheDocument();
  });

  it("discarding a landed proposal returns to idle without applying", async () => {
    stubRefine({
      proposals: [{ id: "p1", jobId: "job-1", status: "submitted", proposal }],
    });
    render(<RefinePanel channelId="ch-1" channelName="90s Action" />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(screen.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(screen.getByRole("button", { name: /^refine$/i }));

    await screen.findByRole("button", { name: /apply changes/i });
    await userEvent.click(screen.getByRole("button", { name: /discard/i }));

    expect(screen.queryByText("Predator")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("What to change")).not.toBeInTheDocument();
  });

  it("cancel from the open textarea returns to idle", async () => {
    stubRefine({ proposals: [] });
    render(<RefinePanel channelId="ch-1" channelName="90s Action" />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(screen.getByLabelText("What to change"), "something");
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByLabelText("What to change")).not.toBeInTheDocument();
  });

  // A generation FAILURE (the model errored/timed out mid-run — a `failed` SSE phase,
  // never a proposal) must not dead-end the panel: it drops back to the form with an
  // inline notice and the typed change preserved, so retry is one click away. This is the
  // "inline error + keep the text" behaviour, not a toast-and-collapse.
  it("surfaces a generation failure inline and keeps the typed change", async () => {
    vi.stubGlobal("EventSource", MockEventSource);
    // The queue never lands a proposal for this job — a failed run produces none.
    stubRefine({ proposals: [] });
    render(<RefinePanel channelId="ch-1" channelName="90s Action" />, { wrapper: makeEventfulWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(screen.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(screen.getByRole("button", { name: /^refine$/i }));

    // The worker fails the job → a `failed` suggestion frame for this run's job id.
    act(() => MockEventSource.last?.emit("suggestion", { jobId: "job-1", phase: "failed" }));

    // Back on the form, with a recoverable error and the text intact — no diff, no Apply.
    expect(await screen.findByText(/couldn't complete/i)).toBeInTheDocument();
    expect(screen.getByLabelText("What to change")).toHaveValue("add more Schwarzenegger");
    expect(screen.queryByRole("button", { name: /apply changes/i })).not.toBeInTheDocument();
    // And the retry affordance is right there.
    expect(screen.getByRole("button", { name: /^refine$/i })).toBeEnabled();
  });
});
