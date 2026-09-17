import type { FillerReadinessDTO } from "@loomarr/api";
import { getFillerReadinessMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerOverview, readinessAction } from "./filler-overview";

const readiness = (over: Partial<FillerReadinessDTO> = {}): FillerReadinessDTO => {
  const { repairs, ...rest } = over;
  return {
    ready: true,
    nextAction: "none",
    repairs: repairs ?? { count: 0 },
    fetch: { enabled: true, catalogClips: 25 },
    pipeline: {
      runnable: 0,
      scheduled: 0,
      inProgress: 0,
      needsDecision: 0,
      recoverable: 0,
      ready: 25,
      complete: 0,
      rejected: 0,
      dismissed: 0,
    },
    pool: { clips: 25, breakBody: 20, eligible: 18, untagged: 0, channels: [] },
    acquisitions: [],
    ...rest,
  };
};

const show = (coverage: FillerReadinessDTO = readiness()) => {
  server.use(getFillerReadinessMockHandler(coverage));
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
  it.each([
    ["enable_fetch", "downloads"],
    ["free_catalog_capacity", "storage"],
    ["free_disk_capacity", "storage"],
  ] as const)("opens the exact settings task for %s", (nextAction, section) => {
    expect(readinessAction(readiness({ ready: false, nextAction }))).toMatchObject({ section });
  });
  it.each([
    ["enable_fetch", "Turn on automatic sourcing", "Review automation", "/filler/settings/$section"],
    [
      "free_catalog_capacity",
      "Make room in the filler catalog",
      "Review limits",
      "/filler/settings/$section",
    ],
    ["free_disk_capacity", "Make room for more filler", "Review limits", "/filler/settings/$section"],
    ["retry_acquisition", "A download needs another try", "Open diagnostics", "/filler/manage"],
    ["retry_failed_work", "Some filler can be retried", "Open diagnostics", "/filler/manage"],
    ["review_incoming", "A few clips need your help", "Review clips", "/filler/incoming"],
    ["add_filler", "Add filler to get started", "Open sources", "/filler/sources"],
    [
      "improve_channel_coverage",
      "A channel needs better filler coverage",
      "Browse library",
      "/filler/library",
    ],
  ] as const)("presents the server-ranked %s action", (nextAction, title, label, to) => {
    expect(readinessAction(readiness({ ready: false, nextAction, actionCount: 2 }))).toMatchObject({
      title,
      label,
      to,
    });
  });

  it("renders no action when the readiness projection ranks none", () => {
    expect(readinessAction(readiness())).toBeUndefined();
  });

  it("renders the server-owned healthy answer without inventing an action", async () => {
    show();
    expect(await screen.findByText("Filler is working on its own")).toBeInTheDocument();
    expect(screen.getByText("Working automatically")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /diagnostics|review clips/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/admission|admitted|readiness projection/i)).not.toBeInTheDocument();
  });

  it("uses workspace readiness rather than a healthy admission subset", async () => {
    show(readiness({ ready: false, nextAction: "add_filler" }));

    expect(await screen.findByText("Add filler to get started")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open sources" })).toHaveAttribute("href", "/filler/sources");
    expect(screen.queryByText("Filler is working on its own")).not.toBeInTheDocument();
    expect(screen.queryByText("Working automatically")).not.toBeInTheDocument();
  });

  it("uses the current needs-help action without resurfacing legacy decision counts", async () => {
    show(readiness({ ready: false, nextAction: "review_incoming", actionCount: 4 }));

    expect(await screen.findByText("A few clips need your help")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Review clips" })).toHaveAttribute("href", "/filler/incoming");
    expect(screen.queryByRole("heading", { name: "Admission summary" })).not.toBeInTheDocument();
    expect(screen.queryByText("Needs judgment")).not.toBeInTheDocument();
    expect(screen.queryByText(/classified safely without a person/i)).not.toBeInTheDocument();
  });

  it("routes operational recovery to diagnostics, never the review queue", async () => {
    show(readiness({ ready: false, nextAction: "retry_failed_work", actionCount: 2 }));
    expect(await screen.findByText("Some filler can be retried")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open diagnostics" })).toHaveAttribute("href", "/filler/manage");
  });

  it("keeps channel coverage separate from admission health", async () => {
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
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
    expect(await screen.findByText("6m playable · 12 clips")).toBeInTheDocument();
    expect(screen.getByText("3 categories · 7 brands")).toBeInTheDocument();
  });
});
