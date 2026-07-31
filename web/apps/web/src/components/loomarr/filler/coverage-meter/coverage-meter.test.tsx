import type { CoverageDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { CoverageMeter } from "./coverage-meter";

// The "Find clips" CTA is a TanStack `Link`, which needs a RouterProvider even in isolation —
// the same harness connection-block's tests use.
//
// ⚠ Two harness facts, each of which cost a debugging round here:
//   1. It registers only the AppShell nav paths. "/channels" is NOT one — the channels surface
//      is "/guide" since the V14 rename — and an unregistered path renders TanStack's "Not
//      Found" instead of the component.
//   2. It mounts ASYNC, so the FIRST query in each test must be findBy*. A synchronous
//      getByText runs before the router has rendered and fails with "unable to find", which
//      looks like a component bug and is not one. connection-block's tests record the same.
const renderMeter = (ui: ReactElement) => render(<RouterHarness content={ui} initialPath="/guide" />);

const full: CoverageDTO = {
  level: "exact",
  total: 9,
  rungs: [
    { level: "exact", clips: 4 },
    { level: "widened", clips: 5 },
    { level: "audience", clips: 9 },
  ],
};

describe("CoverageMeter", () => {
  it("names the rung a break would actually be filled from", async () => {
    renderMeter(<CoverageMeter coverage={full} />);
    expect(await screen.findByText("Exact match")).toBeInTheDocument();
  });

  // Each rung's own count is rendered verbatim. The meter's claim is that it shows what the
  // ladder computed, so a derived-looking number that nobody can trace back is the thing to
  // avoid — these are the server's integers.
  it("renders every rung with its own clip count", async () => {
    renderMeter(<CoverageMeter coverage={full} />);
    expect(await screen.findByText("Exact era + audience")).toBeInTheDocument();
    expect(screen.getByText("Same decade")).toBeInTheDocument();
    expect(screen.getByText("Any era, right audience")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  // ⚠ The total is the widest rung, not a sum. 4+5+9 would claim 18 eligible commercials for a
  // catalog holding 9 — the rungs nest, so adding them counts one clip up to three times.
  it("reports the widest rung as the total, not the sum", async () => {
    renderMeter(<CoverageMeter coverage={full} />);
    expect(await screen.findByText("9 eligible commercials")).toBeInTheDocument();
    expect(screen.queryByText(/18 eligible/)).not.toBeInTheDocument();
  });

  // ⚠ A skipped rung is absent from the server's response, and must stay absent here. Drawing
  // it at 0 reads as a catalog gap to go fix rather than the strict-era setting the operator
  // chose.
  it("omits a rung the channel's policy skips rather than drawing it at zero", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{
          level: "exact",
          total: 9,
          rungs: [
            { level: "exact", clips: 4 },
            { level: "audience", clips: 9 },
          ],
        }}
      />,
    );
    await screen.findByText("Exact match");
    expect(screen.queryByText("Same decade")).not.toBeInTheDocument();
  });

  // bumper_card is the honest "nothing fits" answer — distinct from a zero that reads as
  // still-loading, and the one state an operator most needs named.
  it("says plainly when nothing in the catalog fits", async () => {
    renderMeter(<CoverageMeter coverage={{ level: "bumper_card", total: 0, rungs: [] }} />);
    expect(await screen.findByText("No commercials")).toBeInTheDocument();
    expect(screen.getByText("No eligible commercials for this channel.")).toBeInTheDocument();
  });

  // ⚠ A REGRESSION TEST for a crash this component shipped with for about ten minutes:
  // `LEVEL_COPY[coverage.level]` returned undefined and the read of `.tone` took down the whole
  // Filler section — five unrelated ChannelFiller tests went red at once. The trigger there was
  // a stub payload, but the real one is version skew: a server that adds a ladder rung sends a
  // level this build has never heard of, and the generated union only updates when someone
  // regenerates. Degrading beats crashing.
  it("degrades instead of crashing on a level it does not recognise", async () => {
    renderMeter(
      <CoverageMeter
        // Cast, because the whole point is a value outside the generated union.
        coverage={{ level: "sideways" as CoverageDTO["level"], total: 2, rungs: [] }}
      />,
    );
    expect(await screen.findByText("Coverage unavailable")).toBeInTheDocument();
  });

  // Same reasoning one level down: an unknown RUNG renders its raw level rather than a blank
  // row, so a skewed client still shows that something is there.
  it("falls back to the raw level for an unrecognised rung", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{
          level: "exact",
          total: 3,
          rungs: [{ level: "sideways" as CoverageDTO["level"], clips: 3 }],
        }}
      />,
    );
    expect(await screen.findByText("sideways")).toBeInTheDocument();
  });

  // F4 gap flagging (V17d). ⚠ "Thin" is the LADDER's answer, not a clip count: a channel with
  // three perfectly-matched commercials is not asking for help, while one with two hundred
  // clips that all miss its era is. A count threshold gets both cases backwards.
  it("offers Find clips only when the ladder had to widen", async () => {
    // Separate renders rather than rerender(): each mounts its own RouterHarness, and
    // swapping the harness under a live router is more setup than the assertion needs.
    const healthy = renderMeter(<CoverageMeter coverage={full} />);
    // Wait for the mount BEFORE asserting absence, or this passes on an empty DOM.
    await screen.findByText("Exact match");
    expect(screen.queryByTestId("find-clips")).not.toBeInTheDocument();
    healthy.unmount();

    // Widened: matched material ran out, which is the fixable condition.
    const widened = renderMeter(
      <CoverageMeter coverage={{ level: "widened", total: 6, rungs: [{ level: "widened", clips: 2 }] }} />,
    );
    expect(await screen.findByTestId("find-clips")).toBeInTheDocument();
    widened.unmount();

    // And the worst case obviously offers it.
    renderMeter(<CoverageMeter coverage={{ level: "bumper_card", total: 0, rungs: [] }} />);
    expect(await screen.findByTestId("find-clips")).toBeInTheDocument();
  });

  // A well-stocked channel gets no CTA at all — a banner that always nags is one an operator
  // learns to stop reading.
  it("stays quiet on a healthy channel even with few clips", async () => {
    renderMeter(
      <CoverageMeter coverage={{ level: "exact", total: 3, rungs: [{ level: "exact", clips: 3 }] }} />,
    );
    await screen.findByText("Exact match");
    expect(screen.queryByTestId("find-clips")).not.toBeInTheDocument();
  });

  it("uses the singular for one clip", async () => {
    renderMeter(
      <CoverageMeter coverage={{ level: "exact", total: 1, rungs: [{ level: "exact", clips: 1 }] }} />,
    );
    expect(await screen.findByText("1 eligible commercial")).toBeInTheDocument();
  });
});
