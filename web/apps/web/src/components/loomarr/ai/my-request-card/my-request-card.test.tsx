import type { ProposalDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MyRequestCard } from "./my-request-card";

const base = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
  ({
    id: "p1",
    jobId: "j1",
    status: "submitted",
    createdBy: "u-kid",
    proposal: {
      intent: { description: "90s action night" },
      lineup: [{ name: "Heat", mediaType: "movie", inLibrary: true }],
      acquisitions: [{ name: "Con Air", mediaType: "movie", inLibrary: false }],
    },
    ...over,
  }) as ProposalDTO;

describe("MyRequestCard", () => {
  it("shows the intent and where the request stands", () => {
    render(<MyRequestCard proposal={base()} />);
    expect(screen.getByText("90s action night")).toBeInTheDocument();
    expect(screen.getByText("Waiting for approval")).toBeInTheDocument();
    expect(screen.getByText(/2 titles/)).toBeInTheDocument();
  });

  // ⚠ An edited approval is a DISTINCT outcome from a plain one. "Approved" alone would hide
  // that the lineup someone receives is not the lineup they asked for — which is precisely the
  // case V25b exists to make explicable.
  it("distinguishes approved-with-changes from plain approved", () => {
    const { unmount } = render(<MyRequestCard proposal={base({ status: "approved" })} />);
    expect(screen.getByText("Approved")).toBeInTheDocument();
    unmount();

    render(
      <MyRequestCard
        proposal={base({ status: "approved", modSummary: "dropped 2, added 1", approvedBy: "boss" })}
      />,
    );
    expect(screen.getByText("Approved with changes")).toBeInTheDocument();
  });

  // The phase gate: *"CHANGED BY …"* renders. `modSummary` is generated server-side, so it is a
  // record of what happened rather than a claim someone typed; `approvedBy` names who.
  it("renders CHANGED BY with the server-generated summary", () => {
    render(
      <MyRequestCard
        proposal={base({ status: "approved", modSummary: "dropped 2, added 1", approvedBy: "boss" })}
      />,
    );
    expect(screen.getByText(/Changed by boss/)).toBeInTheDocument();
    expect(screen.getByText(/dropped 2, added 1/)).toBeInTheDocument();
  });

  // Provenance with no author is still worth showing — but it must not read as "changed by
  // undefined".
  it("still reports a change when the approver is unknown", () => {
    render(<MyRequestCard proposal={base({ status: "approved", modSummary: "dropped 1" })} />);
    expect(screen.getByText("Changed")).toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });

  // V25b captured the note and nothing could display it — the DTO didn't even carry it off the
  // server. This is the half of the feature that is about people rather than data.
  it("shows the approver's note", () => {
    render(
      <MyRequestCard
        proposal={base({ status: "approved", modSummary: "swapped 1", note: "we already have that one" })}
      />,
    );
    expect(screen.getByText("we already have that one")).toBeInTheDocument();
  });

  // The phase gate: the denial line shows. A member told only "not approved" has learned
  // nothing and will submit the same intent again.
  it("shows the denial reason", () => {
    render(<MyRequestCard proposal={base({ status: "denied", denyReason: "over the cap this week" })} />);
    expect(screen.getByText("Not approved")).toBeInTheDocument();
    expect(screen.getByText("over the cap this week")).toBeInTheDocument();
  });

  // A denial with no reason must say so plainly rather than rendering an empty line that reads
  // like a rendering bug.
  it("says so when a denial carries no reason", () => {
    render(<MyRequestCard proposal={base({ status: "denied" })} />);
    expect(screen.getByText("No reason was given.")).toBeInTheDocument();
  });

  it("survives a proposal with no intent description", () => {
    render(<MyRequestCard proposal={base({ proposal: {} as ProposalDTO["proposal"] })} />);
    expect(screen.getByText("Suggested lineup")).toBeInTheDocument();
  });
});
