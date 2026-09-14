import type { Proposal } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { ProposalReview } from "./proposal-review";

const renderReview = (ui: ReactElement) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
};

const proposal: Proposal = {
  channelName: "Friday Night Action",
  intent: { description: "90s action movies" },
  rationale: "A high-energy 90s action block.",
  lineup: [
    {
      name: "Heat",
      year: 1995,
      mediaType: "movie",
      tmdbId: 949,
      inLibrary: true,
      rationale: "A grounded match for the requested era and tone.",
    },
  ],
  acquisitions: [{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false }],
  alternates: [{ name: "Face/Off", mediaType: "movie", tmdbId: 754, inLibrary: false }],
  scores: {
    version: 1,
    themeFit: 1,
    availabilityRatio: 0.5,
    eraBalance: null,
    theme: {
      status: "supported",
      basis: "qualifiers",
      assessedItems: 2,
      unknownItems: 0,
      qualifiers: [{ term: "action", supportedItems: 2 }],
    },
    era: { status: "not_requested", assessedItems: 0, matchingItems: 0, unknownItems: 0 },
  },
  trace: {
    version: 1,
    surfacedTotal: 2,
    recordedTotal: 2,
    truncated: false,
    candidates: [
      {
        key: "movie:tmdb:949",
        name: "Heat",
        ownership: "library",
        constraints: { request: true, era: true },
        disposition: "selected",
        reason: "selected",
      },
    ],
  },
};

describe("ProposalReview", () => {
  it("leads with a calm channel summary and plain availability", () => {
    renderReview(<ProposalReview proposal={proposal} selfService />);
    expect(screen.getByRole("heading", { name: "Review your channel" })).toBeInTheDocument();
    expect(screen.getByText("Friday Night Action")).toBeInTheDocument();
    expect(screen.getByText("2", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("Ready now")).toBeInTheDocument();
    expect(screen.getByText("Needs adding")).toBeInTheDocument();
    expect(screen.getByText("Backups")).toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  });

  it("lets an admin choose titles and creates with the exact edited count", async () => {
    const onEdit = vi.fn();
    const onApprove = vi.fn();
    const view = renderReview(
      <ProposalReview
        proposal={proposal}
        status="submitted"
        selfService
        onEdit={onEdit}
        onApprove={onApprove}
      />,
    );

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    expect(onEdit).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:949"] });
    view.rerender(
      <QueryClientProvider client={new QueryClient()}>
        <ProposalReview
          proposal={proposal}
          status="partially-edited"
          selfService
          edit={{ drop: ["movie:tmdb:949"] }}
          onEdit={onEdit}
          onApprove={onApprove}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByText(/with 1 title/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Create channel" }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("prevents creating an empty channel", () => {
    renderReview(
      <ProposalReview
        proposal={proposal}
        status="submitted"
        selfService
        edit={{ drop: ["movie:tmdb:949", "movie:tmdb:1701"] }}
        onEdit={vi.fn()}
        onApprove={vi.fn()}
      />,
    );
    expect(screen.getByText("Choose at least one title")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create channel" })).toBeDisabled();
  });

  it("keeps member review read-only and labels its state", () => {
    renderReview(<ProposalReview proposal={proposal} status="submitted" />);
    expect(screen.getByText("Sent for approval")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create channel" })).not.toBeInTheDocument();
  });

  it("puts deterministic evidence behind progressive disclosure", async () => {
    renderReview(<ProposalReview proposal={proposal} />);
    const evidence = screen.getByText("How Loomarr chose these titles").closest("details");
    expect(evidence).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("How Loomarr chose these titles"));
    expect(evidence).toHaveAttribute("open");
    await userEvent.click(screen.getByText("See the catalog decisions"));
    expect(screen.getByText(/included · matched request, era/)).toBeInTheDocument();
  });

  it("calls out a partial interpretation in plain language", () => {
    const onEditRequest = vi.fn();
    renderReview(
      <ProposalReview
        proposal={{
          ...proposal,
          scores: {
            ...proposal.scores,
            theme: {
              status: "partial",
              basis: "qualifiers",
              assessedItems: 2,
              unknownItems: 0,
              qualifiers: [{ term: "xqz", supportedItems: 0 }],
            },
          },
        }}
        onEditRequest={onEditRequest}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Double-check the fit");
    expect(screen.getByRole("status")).toHaveTextContent("Review the titles below");
    expect(screen.getByRole("status")).not.toHaveTextContent("xqz");
  });

  it("uses discard language for an admin reviewing their own draft", async () => {
    const onDeny = vi.fn();
    renderReview(
      <ProposalReview
        proposal={proposal}
        status="submitted"
        selfService
        onApprove={vi.fn()}
        onDeny={onDeny}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(onDeny).toHaveBeenCalledWith();
  });
});
