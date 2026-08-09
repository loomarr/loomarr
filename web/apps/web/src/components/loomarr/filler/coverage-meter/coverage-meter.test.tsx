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

// ⚠ Every CoverageDTO carries the per-setting breakdown (V51f). Most tests here are about the
// RUNG rendering, so they use a HEALTHY breakdown — nothing at zero — which keeps the diagnosis
// panel out of the way and leaves those assertions testing what they were written to test.
// The breakdown gets its own tests below rather than perturbing every existing one.
const HEALTHY: CoverageDTO["criteria"] = [
  { criterion: "era", clips: 9 },
  { criterion: "audience", clips: 9 },
  { criterion: "category", clips: 9 },
  { criterion: "kind", clips: 9 },
  { criterion: "duration", clips: 9 },
  { criterion: "quality", clips: 9 },
];

const full: CoverageDTO = {
  level: "exact",
  total: 9,
  criteria: HEALTHY,
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
    expect(screen.getByText("A decade either side")).toBeInTheDocument();
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

  // ⚠ **The component renders the rungs it is GIVEN and never invents a missing one.**
  //
  // This test used to be about `EraStrict`, which dropped the widened rung server-side — a field
  // set in tests and nowhere else, deleted in V51f, so every rung now always arrives. The
  // assertion survives its premise because the real property was never about that setting: a
  // component that filled in an absent rung at zero would be manufacturing a catalog gap the
  // server never reported, and would do it for any future reason a rung goes missing (an older
  // server, a partial response) rather than only for the one flag that used to cause it.
  it("renders only the rungs the server sent, never a fabricated zero", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{
          level: "exact",
          total: 9,
          criteria: HEALTHY,
          rungs: [
            { level: "exact", clips: 4 },
            { level: "audience", clips: 9 },
          ],
        }}
      />,
    );
    await screen.findByText("Exact match");
    expect(screen.queryByText("A decade either side")).not.toBeInTheDocument();
  });

  // bumper_card is the honest "nothing fits" answer — distinct from a zero that reads as
  // still-loading, and the one state an operator most needs named.
  it("says plainly when nothing in the catalog fits", async () => {
    renderMeter(
      <CoverageMeter coverage={{ level: "bumper_card", total: 0, rungs: [], criteria: HEALTHY }} />,
    );
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
        coverage={{ level: "sideways" as CoverageDTO["level"], total: 2, rungs: [], criteria: HEALTHY }}
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
          criteria: HEALTHY,
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
      <CoverageMeter
        coverage={{ level: "widened", total: 6, rungs: [{ level: "widened", clips: 2 }], criteria: HEALTHY }}
      />,
    );
    expect(await screen.findByTestId("find-clips")).toBeInTheDocument();
    widened.unmount();

    // And the worst case obviously offers it.
    renderMeter(
      <CoverageMeter coverage={{ level: "bumper_card", total: 0, rungs: [], criteria: HEALTHY }} />,
    );
    expect(await screen.findByTestId("find-clips")).toBeInTheDocument();
  });

  // A well-stocked channel gets no CTA at all — a banner that always nags is one an operator
  // learns to stop reading.
  it("stays quiet on a healthy channel even with few clips", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{ level: "exact", total: 3, rungs: [{ level: "exact", clips: 3 }], criteria: HEALTHY }}
      />,
    );
    await screen.findByText("Exact match");
    expect(screen.queryByTestId("find-clips")).not.toBeInTheDocument();
  });

  it("uses the singular for one clip", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{ level: "exact", total: 1, rungs: [{ level: "exact", clips: 1 }], criteria: HEALTHY }}
      />,
    );
    expect(await screen.findByText("1 eligible commercial")).toBeInTheDocument();
  });

  // --- the per-setting breakdown (§10 V51f) ---

  // ⚠ **The failure the breakdown exists for.** "Nothing in the catalog fits" reads as "go and
  // acquire more clips"; naming the audience says the catalog is fine and one setting is not.
  // Before this, an operator picking an Audience on an untagged catalog got the first message
  // and no way to reach the second.
  it("names the one setting that is ruling out every commercial", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{
          level: "bumper_card",
          total: 0,
          rungs: [],
          // ⚠ Distinct counts on purpose: with three settings sharing 214 the assertion below
          // matched three nodes and threw. Duplicates also make the fixture read as if the
          // criteria were correlated, which is the one thing they are specifically not.
          criteria: [
            { criterion: "era", clips: 214 },
            { criterion: "audience", clips: 0 },
            { criterion: "category", clips: 112 },
            { criterion: "kind", clips: 198 },
            { criterion: "duration", clips: 205 },
            { criterion: "quality", clips: 211 },
          ],
        }}
      />,
    );
    expect(await screen.findByText("One setting is ruling out every commercial:")).toBeInTheDocument();
    expect(screen.getByText("Audience")).toBeInTheDocument();
    expect(screen.getByText("nothing matches")).toBeInTheDocument();
    // The healthy settings are still listed, with their counts — the contrast is what makes the
    // zero legible as the culprit rather than as one number among six.
    expect(screen.getByText("214 clips")).toBeInTheDocument();
  });

  it("pluralises the heading when more than one setting is empty", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{
          level: "bumper_card",
          total: 0,
          rungs: [],
          criteria: [
            { criterion: "era", clips: 0 },
            { criterion: "audience", clips: 0 },
            { criterion: "category", clips: 5 },
            { criterion: "kind", clips: 5 },
            { criterion: "duration", clips: 5 },
            { criterion: "quality", clips: 5 },
          ],
        }}
      />,
    );
    expect(await screen.findByText("These settings are ruling out every commercial:")).toBeInTheDocument();
  });

  // ⚠ A healthy channel gets no breakdown. Six rows restating that everything is fine is how an
  // operator learns to stop reading this panel — and then misses it on the day it matters.
  it("stays out of the way when the channel resolves exactly", async () => {
    renderMeter(<CoverageMeter coverage={full} />);
    await screen.findByText("Exact match");
    expect(screen.queryByText(/ruling out every commercial/)).not.toBeInTheDocument();
  });

  // ...and equally when the ladder widened but no single setting is the cause. There is nothing
  // to name, so naming nothing is the honest answer.
  it("shows no breakdown when the channel is thin but no setting is empty", async () => {
    renderMeter(
      <CoverageMeter
        coverage={{ level: "widened", total: 6, rungs: [{ level: "widened", clips: 6 }], criteria: HEALTHY }}
      />,
    );
    await screen.findByText("Widened the era");
    expect(screen.queryByText(/ruling out every commercial/)).not.toBeInTheDocument();
  });
});
