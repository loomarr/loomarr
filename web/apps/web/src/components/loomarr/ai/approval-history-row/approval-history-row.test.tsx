import type { ProposalDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ApprovalHistoryRow } from "./approval-history-row";

const base = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
  ({
    id: "p1",
    jobId: "j1",
    status: "approved",
    createdBy: "kid",
    approvedBy: "boss",
    approvedAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    proposal: { intent: { description: "Saturday morning cartoons" } },
    ...over,
  }) as ProposalDTO;

describe("ApprovalHistoryRow", () => {
  it("shows what was decided, by whom, and when", () => {
    render(<ApprovalHistoryRow proposal={base()} />);

    expect(screen.getByText("Saturday morning cartoons")).toBeInTheDocument();
    expect(screen.getByText("Approved")).toBeInTheDocument();
    expect(screen.getByText(/Approved by boss/)).toBeInTheDocument();
    expect(screen.getByText("2h ago")).toBeInTheDocument();
  });

  // The gate's third clause: history rows carry `approvedAt`. It needed its own column — three
  // subsystems write `updated_at`, so a re-curation would have silently reordered this trail.
  it("renders the approval time", () => {
    render(
      <ApprovalHistoryRow proposal={base({ approvedAt: new Date(Date.now() - 5 * 60_000).toISOString() })} />,
    );
    expect(screen.getByText("5m ago")).toBeInTheDocument();
  });

  it("distinguishes approved-with-changes and names what changed", () => {
    render(<ApprovalHistoryRow proposal={base({ modSummary: "dropped 2, added 1" })} />);

    expect(screen.getByText("Approved with changes")).toBeInTheDocument();
    expect(screen.getByText(/dropped 2, added 1/)).toBeInTheDocument();
  });

  // A denial has no approval time. Showing one would put a date on a decision that never
  // happened — the same reason the DTO omits the field rather than sending a zero.
  it("shows a denial and its reason, with no approval time", () => {
    render(
      <ApprovalHistoryRow
        proposal={base({
          status: "denied",
          approvedBy: undefined,
          approvedAt: undefined,
          denyReason: "over the cap this week",
        })}
      />,
    );

    expect(screen.getByText("Denied")).toBeInTheDocument();
    expect(screen.getByText("over the cap this week")).toBeInTheDocument();
    expect(screen.queryByText(/ago/)).not.toBeInTheDocument();
  });

  it("survives a proposal with no intent description", () => {
    render(<ApprovalHistoryRow proposal={base({ proposal: {} as ProposalDTO["proposal"] })} />);
    expect(screen.getByText("Suggested lineup")).toBeInTheDocument();
  });
});
