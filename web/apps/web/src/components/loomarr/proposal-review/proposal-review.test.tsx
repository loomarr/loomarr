import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ProposalReview } from "./proposal-review";
import type { ProposalView } from "./proposal-review.type";

const proposal: ProposalView = {
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
  scores: { themeFit: 0.88, availabilityRatio: 0.5 },
};

describe("ProposalReview", () => {
  it("separates in-library lineup from acquisitions and shows scores", () => {
    render(<ProposalReview proposal={proposal} />);
    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("In library")).toBeInTheDocument();
    expect(screen.getByText("Will acquire")).toBeInTheDocument();
    expect(screen.getByText("88%")).toBeInTheDocument();
  });

  it("gates on approve for an actionable proposal", () => {
    const onApprove = vi.fn();
    render(<ProposalReview proposal={proposal} status="submitted" onApprove={onApprove} />);
    fireEvent.click(screen.getByRole("button", { name: /approve & acquire/i }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("offers edit-via-search per item", () => {
    const onEditItem = vi.fn();
    render(<ProposalReview proposal={proposal} onEditItem={onEditItem} />);
    fireEvent.click(screen.getByRole("button", { name: /swap heat/i }));
    expect(onEditItem).toHaveBeenCalledWith(proposal.lineup[0]);
  });

  it("retires the actions once approved", () => {
    render(<ProposalReview proposal={proposal} status="approved" />);
    expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
  });
});
