import type { FillerReadinessDTO } from "@loomarr/api";
import { getFillerReadinessMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerOverview } from "./filler-overview";

const readiness = (over: Partial<FillerReadinessDTO> = {}): FillerReadinessDTO => ({
  ready: true,
  nextAction: "none",
  fetch: { enabled: true, catalogClips: 25 },
  pipeline: {
    runnable: 0,
    scheduled: 0,
    inProgress: 0,
    needsDecision: 0,
    recoverable: 0,
    admitted: 25,
    rejected: 0,
    dismissed: 0,
  },
  pool: { clips: 25, commercials: 20, eligible: 18, untagged: 0, channels: [] },
  acquisitions: [],
  ...over,
});

const show = (body: FillerReadinessDTO) => {
  server.use(getFillerReadinessMockHandler(body));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <RouterHarness
      content={
        <QueryClientProvider client={client}>
          <FillerOverview />
        </QueryClientProvider>
      }
      initialPath="/filler"
    />,
  );
};

describe("FillerOverview", () => {
  it("answers readiness plainly when unattended filler is healthy", async () => {
    show(readiness());

    expect(await screen.findByText("Filler is working on its own")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("keeps machine work, operator decisions, recovery, and terminal audit distinct", async () => {
    show(
      readiness({
        ready: false,
        nextAction: "retry_failed_work",
        actionCount: 2,
        pipeline: {
          runnable: 2,
          scheduled: 1,
          inProgress: 3,
          needsDecision: 4,
          recoverable: 2,
          admitted: 18,
          rejected: 5,
          dismissed: 1,
        },
      }),
    );

    expect(await screen.findByText("Some prepared filler can be recovered")).toBeInTheDocument();
    expect(screen.getByText("6")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("2", { selector: "p" })).toBeInTheDocument();
    expect(screen.getByText("Rejected or dismissed: 6")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Recover filler" })).toHaveAttribute("href", "/filler/attention");
  });

  it("shows usable channel duration and grounded variety with a direct fix path", async () => {
    show(
      readiness({
        ready: false,
        nextAction: "improve_channel_coverage",
        channelId: "ch-42",
        pool: {
          clips: 25,
          commercials: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            {
              channelId: "ch-42",
              name: "Saturday Mornings",
              number: 42,
              level: "widened",
              total: 12,
              durationMs: 360_000,
              categories: 3,
              brands: 7,
            },
          ],
        },
      }),
    );

    expect(await screen.findByText("Improve a channel's filler coverage")).toBeInTheDocument();
    expect(screen.getByText("6m playable · 12 clips")).toBeInTheDocument();
    expect(screen.getByText("3 categories · 7 brands")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Review channel coverage" })).toHaveAttribute(
      "href",
      "/channels/ch-42/filler",
    );
  });

  it("traces a failed acquisition without mixing it into operator decisions", async () => {
    show(
      readiness({
        ready: false,
        nextAction: "retry_acquisition",
        acquisitions: [
          {
            id: "acq-1",
            trigger: "pull",
            status: "error",
            requested: 3,
            fetched: 1,
            skipped: 0,
            failed: 2,
            empty: 0,
            error: "archive request timed out",
            startedAt: "2026-08-23T12:00:00Z",
            updatedAt: "2026-08-23T12:01:00Z",
            outcome: { enrolled: 1, preparing: 0, needsDecision: 0, admitted: 0, rejected: 0, dismissed: 0 },
          },
        ],
      }),
    );

    expect(await screen.findByText("A filler acquisition failed")).toBeInTheDocument();
    expect(screen.getByText("Approved pull")).toBeInTheDocument();
    expect(screen.getByText("archive request timed out")).toBeInTheDocument();
  });
});
