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

  it("states ready, acquisition, policy, and refusal facts before approval", () => {
    renderWithTooltip(
      <ProposalReview
        proposal={{
          ...proposal,
          acquisitions: [],
          policy: {
            audience: { ceiling: "PG-13", unrated: "exclude" },
            scope: { era: { from: 1990, to: 1999 }, runtimeMax: 7_200 },
            ordering: "syndication",
            separation: {
              movieNoRepeat: "168h",
              episodeNoRepeat: "24h",
              seriesMinGap: "4h",
              blockMax: 3,
            },
          },
          refused: [
            {
              item: {
                name: "The Rock",
                year: 1996,
                mediaType: "movie",
                inLibrary: true,
                officialRating: "R",
              },
              reason: "over_ceiling",
            },
            {
              item: {
                name: "Die Hard",
                year: 1988,
                mediaType: "movie",
                inLibrary: true,
              },
              reason: "out_of_scope",
            },
          ],
        }}
      />,
    );

    expect(screen.getByText("1 ready now")).toBeInTheDocument();
    expect(screen.getByText("0 to acquire")).toBeInTheDocument();
    expect(screen.getByText("Audience · PG-13 · Unrated excluded")).toBeInTheDocument();
    expect(screen.getByText("Scope · 1990–1999 · Up to 120 min")).toBeInTheDocument();
    expect(screen.getByText("Ordering · Syndication")).toBeInTheDocument();
    expect(
      screen.getByText("Separation · Movies 7 days · Episodes 1 day · Series 4 hours · Block limit 3"),
    ).toBeInTheDocument();
    expect(screen.getByText("The Rock")).toBeInTheDocument();
    expect(screen.getByText(/Rated R, above the PG-13 audience limit\./)).toBeInTheDocument();
    expect(screen.getByText("Die Hard")).toBeInTheDocument();
    expect(screen.getByText(/Outside this channel's scope\./)).toBeInTheDocument();
  });

  it("keeps each title rationale behind a native disclosure", () => {
    renderWithTooltip(<ProposalReview proposal={proposal} />);

    const disclosure = screen.getByText("Why Heat fits").closest("details");
    expect(disclosure).not.toHaveAttribute("open");
    expect(screen.getByText("Peak-era Mann.")).not.toBeVisible();

    fireEvent.click(screen.getByText("Why Heat fits"));
    expect(disclosure).toHaveAttribute("open");
    expect(screen.getByText("Peak-era Mann.")).toBeVisible();
  });

  it("gates on approve for an actionable proposal", () => {
    const onApprove = vi.fn();
    renderWithTooltip(<ProposalReview proposal={proposal} status="submitted" onApprove={onApprove} />);
    fireEvent.click(screen.getByRole("button", { name: /approve & acquire/i }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("renders only decision controls backed by callbacks", () => {
    const { rerender } = renderWithTooltip(<ProposalReview proposal={proposal} status="submitted" />);
    expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /deny/i })).not.toBeInTheDocument();

    rerender(
      <TooltipProvider>
        <ProposalReview proposal={proposal} status="submitted" onApprove={() => {}} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("button", { name: /approve & acquire/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /deny/i })).not.toBeInTheDocument();
  });

  it("announces and locks the approval action while the channel is being created", () => {
    renderWithTooltip(
      <ProposalReview
        proposal={proposal}
        status="submitted"
        busy
        approving
        onApprove={() => {}}
        onDeny={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "Creating channel…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Creating channel…" })).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Deny" })).toBeDisabled();
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
