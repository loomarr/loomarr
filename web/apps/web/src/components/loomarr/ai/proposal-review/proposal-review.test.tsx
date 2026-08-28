import type { Proposal } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ProposalReview } from "./proposal-review";

// The edit-per-item button carries a tooltip, and Radix tooltips need a provider
// ancestor (mounted at the app root in __root.tsx). Wrap renders so the isolated
// component test has one too.
const renderWithTooltip = (ui: ReactElement) => render(<TooltipProvider>{ui}</TooltipProvider>);

const proposal: Proposal = {
  intent: { description: "90s action movies" },
  rationale: "A high-energy 90s action block, front-loaded with the crowd-pleasers.",
  lineup: [
    {
      name: "Heat",
      year: 1995,
      mediaType: "movie",
      inLibrary: true,
      confidence: 0.92,
      rationale: "Peak-era Mann.",
    },
  ],
  acquisitions: [{ name: "Con Air", year: 1997, mediaType: "movie", inLibrary: false, confidence: 0.81 }],
  alternates: [{ name: "Face/Off", mediaType: "movie", inLibrary: false }],
  scores: { themeFit: 0.88, availabilityRatio: 0.5, eraBalance: 0.6, overall: 0.75 },
  trace: {
    version: 1,
    surfacedTotal: 2,
    recordedTotal: 2,
    truncated: false,
    candidates: [
      {
        key: "movie:tmdb: Heat",
        name: "Heat",
        ownership: "library",
        disposition: "selected",
        reason: "selected",
      },
      {
        key: "movie:tmdb: Face",
        name: "Face/Off",
        ownership: "acquisition",
        disposition: "alternate",
        reason: "acquisition_cap",
      },
    ],
  },
};

describe("ProposalReview", () => {
  it("separates in-library lineup from acquisitions and shows scores", () => {
    renderWithTooltip(<ProposalReview proposal={proposal} />);
    expect(screen.getAllByText("Heat").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("In library")).toBeInTheDocument();
    expect(screen.getByText("Will acquire")).toBeInTheDocument();
    expect(screen.getByText("88%")).toBeInTheDocument();
  });

  it("renders deterministic why-this and why-not evidence from the trace", () => {
    renderWithTooltip(<ProposalReview proposal={proposal} />);
    expect(screen.getByRole("heading", { name: "Why this / why not" })).toBeInTheDocument();
    expect(screen.getAllByText("Heat").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("acquisition cap")).toBeInTheDocument();
  });

  it("explains episode selection before approval", () => {
    const curated: Proposal = {
      ...proposal,
      lineup: [
        {
          name: "The Simpsons",
          mediaType: "series",
          tmdbId: 456,
          inLibrary: true,
          episodeSelection: { mode: "highlights" },
        },
      ],
    };
    renderWithTooltip(<ProposalReview proposal={curated} status="submitted" />);
    expect(screen.getByText("Curated highlights")).toBeInTheDocument();
  });

  it("explains a complete episode deck before approval", () => {
    const complete: Proposal = {
      ...proposal,
      lineup: [
        {
          name: "The Simpsons",
          mediaType: "series",
          tmdbId: 456,
          inLibrary: true,
          episodeSelection: { mode: "complete" },
        },
      ],
    };
    renderWithTooltip(<ProposalReview proposal={complete} status="submitted" />);
    expect(screen.getByText("All episodes")).toBeInTheDocument();
  });

  it("explains omitted legacy series selection as the complete deck", () => {
    const legacy: Proposal = {
      ...proposal,
      lineup: [
        {
          name: "The Simpsons",
          mediaType: "series",
          tmdbId: 456,
          inLibrary: true,
        },
      ],
    };
    renderWithTooltip(<ProposalReview proposal={legacy} status="submitted" />);
    expect(screen.getByText("All episodes")).toBeInTheDocument();
  });

  it("explains an unknown legacy series selection as the complete deck", () => {
    const legacy: Proposal = {
      ...proposal,
      lineup: [
        {
          name: "The Simpsons",
          mediaType: "series",
          tmdbId: 456,
          inLibrary: true,
          episodeSelection: { mode: "retired-mode" },
        },
      ],
    };
    renderWithTooltip(<ProposalReview proposal={legacy} status="submitted" />);
    expect(screen.getByText("All episodes")).toBeInTheDocument();
  });

  it("gates on approve for an actionable proposal", () => {
    const onApprove = vi.fn();
    renderWithTooltip(<ProposalReview proposal={proposal} status="submitted" onApprove={onApprove} />);
    fireEvent.click(screen.getByRole("button", { name: /approve & acquire/i }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("offers edit-via-search per item", () => {
    const onEditItem = vi.fn();
    renderWithTooltip(<ProposalReview proposal={proposal} onEditItem={onEditItem} />);
    fireEvent.click(screen.getByRole("button", { name: /edit heat/i }));
    expect(onEditItem).toHaveBeenCalledWith(proposal.lineup?.[0]);
  });

  it("retires the actions once approved", () => {
    renderWithTooltip(<ProposalReview proposal={proposal} status="approved" />);
    expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
  });
});
