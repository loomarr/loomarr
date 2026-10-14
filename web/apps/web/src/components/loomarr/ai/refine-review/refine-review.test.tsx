import type { ChannelPolicy, ProposalItem } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { eraDates } from "@/lib/era-dates";
import { RefineReview } from "./refine-review";
import type { CurrentLineupItem } from "./refine-review.type";

const cartographer: ProposalItem = {
  name: "The Velvet Cartographer",
  year: 1995,
  mediaType: "movie",
  tmdbId: 90000949,
  inLibrary: true,
};
const satelliteChoir: ProposalItem = {
  name: "The Seventh Satellite Choir",
  year: 1991,
  mediaType: "movie",
  tmdbId: 90009426,
  inLibrary: true,
};
const marigold: ProposalItem = {
  name: "Marigold Protocol",
  year: 1987,
  mediaType: "movie",
  tmdbId: 90000106,
  inLibrary: true,
};
const pelicanPoint: ProposalItem = {
  name: "Pelican Point Motel",
  year: 1997,
  mediaType: "movie",
  inLibrary: false,
};

const currentCartographer: CurrentLineupItem = {
  name: "The Velvet Cartographer",
  year: 1995,
  key: "movie:tmdb:90000949",
};
const currentSatelliteChoir: CurrentLineupItem = {
  name: "The Seventh Satellite Choir",
  year: 1991,
  key: "movie:tmdb:90009426",
};

describe("RefineReview", () => {
  it("classifies kept/added/removed by key", () => {
    render(
      <RefineReview
        proposed={[cartographer, marigold]}
        current={[currentCartographer, currentSatelliteChoir]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );

    // The Velvet Cartographer is on both sides → kept.
    expect(screen.getByText("Keeping · 1")).toBeInTheDocument();
    expect(screen.getByText("The Velvet Cartographer")).toBeInTheDocument();

    // Marigold Protocol is new → added.
    expect(screen.getByText("Adding · 1")).toBeInTheDocument();
    expect(screen.getByText("Marigold Protocol")).toBeInTheDocument();

    // The Seventh Satellite Choir dropped off the proposed lineup → removed.
    expect(screen.getByText("Removing · 1")).toBeInTheDocument();
    expect(screen.getByText("The Seventh Satellite Choir")).toBeInTheDocument();
  });

  it("falls back to name+year matching when neither side has a usable id", () => {
    // No tmdbId/tvdbId on the proposed item and no key on the current row — the only
    // case where the fallback should even engage; cartographer/satelliteChoir (both keyed by tmdbId)
    // must never coincidentally collide with an unkeyed current row of the same name.
    // Marigold Protocol (a keyed add) rides along so this scenario isn't indistinguishable
    // from "no changes" — it's the Keeping section specifically under test here.
    const unidentified: ProposalItem = {
      name: "The Velvet Cartographer",
      year: 1995,
      mediaType: "movie",
      inLibrary: true,
    };
    render(
      <RefineReview
        proposed={[unidentified, marigold]}
        current={[{ name: "The Velvet Cartographer", year: 1995 }]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Keeping · 1")).toBeInTheDocument();
    expect(screen.getByText("Adding · 1")).toBeInTheDocument();
    expect(screen.queryByText(/removing/i)).not.toBeInTheDocument();
  });

  it("renders acquisitions under Adding with the downloading note, distinct from library adds", () => {
    render(
      <RefineReview
        proposed={[cartographer]}
        acquisitions={[pelicanPoint]}
        current={[currentCartographer]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Adding (needs downloading) · 1")).toBeInTheDocument();
    expect(screen.getByText("Pelican Point Motel")).toBeInTheDocument();
  });

  it("shows the no-changes state and hides Apply when nothing differs", () => {
    render(
      <RefineReview
        proposed={[cartographer, satelliteChoir]}
        current={[currentCartographer, currentSatelliteChoir]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("No changes. Your channel already matches.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /apply changes/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /discard/i })).toBeInTheDocument();
  });

  it("treats an empty current lineup as everything being added", () => {
    render(
      <RefineReview proposed={[cartographer, marigold]} current={[]} onApply={vi.fn()} onDiscard={vi.fn()} />,
    );
    expect(screen.getByText("Adding · 2")).toBeInTheDocument();
    expect(screen.queryByText(/keeping/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/removing/i)).not.toBeInTheDocument();
  });

  it("shows a programming-policy delta chip for an unpinned field the refine changes", () => {
    // Lineup unchanged (The Velvet Cartographer kept), but the refine changes the era — that policy change used
    // to apply invisibly (the P3 data-loss bug); the chip is the visible half of the fix.
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer]}
        currentPolicy={{ scope: { dates: eraDates({ from: 1980, to: 1989 }) } }}
        proposedPolicy={{ scope: { dates: eraDates({ from: 1990, to: 1999 }) } }}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Programming · 1")).toBeInTheDocument();
    expect(screen.getByText("Era")).toBeInTheDocument();
    expect(screen.getByText(/1980/)).toBeInTheDocument();
    expect(screen.getByText(/1990/)).toBeInTheDocument();
    // A policy-only change is still a change → Apply is offered, not "no changes".
    expect(screen.getByRole("button", { name: /apply changes/i })).toBeInTheDocument();
  });

  // Per-axis dates are not an era: the row summarises the windows instead of reading "Any".
  it("summarises per-axis programming dates rather than calling them Any", () => {
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer]}
        currentPolicy={{ scope: { dates: eraDates({ from: 1980, to: 1989 }) } }}
        proposedPolicy={{
          scope: {
            dates: {
              movieRelease: [
                { from: 1985, to: 1995 },
                { from: 2001, to: 2003 },
              ],
              seriesAiring: [{ from: 1990, to: 1999 }],
            },
          },
        }}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Programming dates")).toBeInTheDocument();
    expect(
      screen.getByText("Movie release 1985–1995, 2001–2003 · Episode airing 1990–1999"),
    ).toBeInTheDocument();
  });

  // V19: the model's why-it-fits was already on ProposalItem and already rendered by
  // ProposalReview — the refine diff built its rows from the same items and dropped it
  // one function before render. "Adding: Marigold Protocol" with no reason is the refine asking
  // for trust it hasn't earned.
  it("shows the model's reason on an added title", () => {
    const withReason: ProposalItem = {
      ...marigold,
      rationale: "Same era, same energy as The Velvet Cartographer.",
    };
    render(
      <RefineReview
        proposed={[cartographer, withReason]}
        current={[currentCartographer]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Same era, same energy as The Velvet Cartographer.")).toBeInTheDocument();
  });

  it("shows the reason on an acquisition too", () => {
    const withReason: ProposalItem = { ...pelicanPoint, rationale: "Fills the late-block slot." };
    render(
      <RefineReview
        proposed={[cartographer]}
        acquisitions={[withReason]}
        current={[currentCartographer]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Fills the late-block slot.")).toBeInTheDocument();
  });

  it("does not explain a removal — the model didn't choose it", () => {
    // A removed title's row comes from the CURRENT lineup, which carries no model
    // rationale; and justifying a removal isn't the model's to give. The assertion
    // guards against a future refactor rendering a stale reason on the wrong row.
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer, currentSatelliteChoir]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Removing · 1")).toBeInTheDocument();
    expect(screen.queryByText(/energy|slot|fills/i)).not.toBeInTheDocument();
  });

  it("shows a separation delta when ONLY the no-repeat window changes", () => {
    // C3′: MergeFromProposal refreshes `separation` exactly like era/audience/ordering/seasonal,
    // but policyDeltas didn't diff it — so a refine could widen or drop a no-repeat window with
    // nothing shown before Approve. Lineup and every other policy field are identical here, so
    // the ONLY thing that can produce a delta is separation.
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer]}
        currentPolicy={{ separation: { movieNoRepeat: "168h" } } as ChannelPolicy}
        proposedPolicy={{ separation: { movieNoRepeat: "24h" } } as ChannelPolicy}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Programming · 1")).toBeInTheDocument();
    expect(screen.getByText("No-repeat · movies")).toBeInTheDocument();
    expect(screen.getByText(/168h/)).toBeInTheDocument();
    expect(screen.getByText(/24h/)).toBeInTheDocument();
  });

  it("does not report a separation delta when the backend just pads zero units", () => {
    // "168h" and "168h0m0s" are the same window — Duration.String() pads. Comparing raw
    // strings would show a phantom diff on every refine that touched nothing.
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer]}
        currentPolicy={{ separation: { movieNoRepeat: "168h" } } as ChannelPolicy}
        proposedPolicy={{ separation: { movieNoRepeat: "168h0m0s" } } as ChannelPolicy}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.queryByText(/no-repeat/i)).not.toBeInTheDocument();
  });

  it("marks a pinned field as kept — the refine cannot overwrite an operator's setting", () => {
    render(
      <RefineReview
        proposed={[cartographer]}
        current={[currentCartographer]}
        currentPolicy={{ scope: { dates: eraDates({ from: 1980, to: 1989 }) }, operatorSet: ["scope"] }}
        proposedPolicy={{ scope: { dates: eraDates({ from: 1990, to: 1999 }) } }}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText(/you set this/i)).toBeInTheDocument();
  });

  it("fires onApply and onDiscard", () => {
    const onApply = vi.fn();
    const onDiscard = vi.fn();
    render(
      <RefineReview
        proposed={[cartographer, marigold]}
        current={[currentCartographer]}
        onApply={onApply}
        onDiscard={onDiscard}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /apply changes/i }));
    fireEvent.click(screen.getByRole("button", { name: /discard/i }));
    expect(onApply).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
  });

  it("disables both actions while busy", () => {
    render(
      <RefineReview
        proposed={[cartographer, marigold]}
        current={[currentCartographer]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
        busy
      />,
    );
    expect(screen.getByRole("button", { name: /apply changes/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /discard/i })).toBeDisabled();
  });
});
