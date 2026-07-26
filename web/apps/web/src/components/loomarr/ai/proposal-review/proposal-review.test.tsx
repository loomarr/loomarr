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
};

describe("ProposalReview", () => {
  it("separates in-library lineup from acquisitions and shows scores", () => {
    renderWithTooltip(<ProposalReview proposal={proposal} />);
    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("In library")).toBeInTheDocument();
    expect(screen.getByText("Will acquire")).toBeInTheDocument();
    expect(screen.getByText("88%")).toBeInTheDocument();
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
