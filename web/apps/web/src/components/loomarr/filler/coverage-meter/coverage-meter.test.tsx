import type { CoverageDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CoverageMeter } from "./coverage-meter";

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
  it("names the rung a break would actually be filled from", () => {
    render(<CoverageMeter coverage={full} />);
    expect(screen.getByText("Exact match")).toBeInTheDocument();
  });

  // Each rung's own count is rendered verbatim. The meter's claim is that it shows what the
  // ladder computed, so a derived-looking number that nobody can trace back is the thing to
  // avoid — these are the server's integers.
  it("renders every rung with its own clip count", () => {
    render(<CoverageMeter coverage={full} />);
    expect(screen.getByText("Exact era + audience")).toBeInTheDocument();
    expect(screen.getByText("Same decade")).toBeInTheDocument();
    expect(screen.getByText("Any era, right audience")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  // ⚠ The total is the widest rung, not a sum. 4+5+9 would claim 18 eligible commercials for a
  // catalog holding 9 — the rungs nest, so adding them counts one clip up to three times.
  it("reports the widest rung as the total, not the sum", () => {
    render(<CoverageMeter coverage={full} />);
    expect(screen.getByText("9 eligible commercials")).toBeInTheDocument();
    expect(screen.queryByText(/18 eligible/)).not.toBeInTheDocument();
  });

  // ⚠ A skipped rung is absent from the server's response, and must stay absent here. Drawing
  // it at 0 reads as a catalog gap to go fix rather than the strict-era setting the operator
  // chose.
  it("omits a rung the channel's policy skips rather than drawing it at zero", () => {
    render(
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
    expect(screen.queryByText("Same decade")).not.toBeInTheDocument();
  });

  // bumper_card is the honest "nothing fits" answer — distinct from a zero that reads as
  // still-loading, and the one state an operator most needs named.
  it("says plainly when nothing in the catalog fits", () => {
    render(<CoverageMeter coverage={{ level: "bumper_card", total: 0, rungs: [] }} />);
    expect(screen.getByText("No commercials")).toBeInTheDocument();
    expect(screen.getByText("No eligible commercials for this channel.")).toBeInTheDocument();
  });

  // ⚠ A REGRESSION TEST for a crash this component shipped with for about ten minutes:
  // `LEVEL_COPY[coverage.level]` returned undefined and the read of `.tone` took down the whole
  // Filler section — five unrelated ChannelFiller tests went red at once. The trigger there was
  // a stub payload, but the real one is version skew: a server that adds a ladder rung sends a
  // level this build has never heard of, and the generated union only updates when someone
  // regenerates. Degrading beats crashing.
  it("degrades instead of crashing on a level it does not recognise", () => {
    render(
      <CoverageMeter
        // Cast, because the whole point is a value outside the generated union.
        coverage={{ level: "sideways" as CoverageDTO["level"], total: 2, rungs: [] }}
      />,
    );
    expect(screen.getByText("Coverage unavailable")).toBeInTheDocument();
  });

  // Same reasoning one level down: an unknown RUNG renders its raw level rather than a blank
  // row, so a skewed client still shows that something is there.
  it("falls back to the raw level for an unrecognised rung", () => {
    render(
      <CoverageMeter
        coverage={{
          level: "exact",
          total: 3,
          rungs: [{ level: "sideways" as CoverageDTO["level"], clips: 3 }],
        }}
      />,
    );
    expect(screen.getByText("sideways")).toBeInTheDocument();
  });

  it("uses the singular for one clip", () => {
    render(<CoverageMeter coverage={{ level: "exact", total: 1, rungs: [{ level: "exact", clips: 1 }] }} />);
    expect(screen.getByText("1 eligible commercial")).toBeInTheDocument();
  });
});
