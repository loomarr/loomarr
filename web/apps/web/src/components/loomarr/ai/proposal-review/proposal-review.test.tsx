import type { Proposal } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
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
  it("leads with the brief, one availability sentence and no dashboard tiles", () => {
    renderReview(<ProposalReview proposal={proposal} selfService onRevise={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Review your channel" })).toBeInTheDocument();
    expect(screen.getByText("Friday Night Action")).toBeInTheDocument();
    expect(screen.getByText("“90s action movies”")).toBeInTheDocument();
    expect(screen.getByText("2 titles · 1 in your library · 1 will be added")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit brief" })).toBeInTheDocument();
    expect(screen.queryByText("selected")).not.toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  });

  it("opens a compact brief editor without hiding the lineup", async () => {
    const onRevise = vi.fn();
    renderReview(<ProposalReview proposal={proposal} selfService onRevise={onRevise} />);
    await userEvent.click(screen.getByRole("button", { name: "Edit brief" }));
    expect(screen.getByLabelText("Channel brief")).toHaveValue("90s action movies");
    expect(within(screen.getByRole("list", { name: "Titles" })).getByText("Heat")).toBeVisible();
    expect(screen.getByRole("button", { name: "Update suggestions" })).toBeVisible();
  });

  it("revises the description without dropping the request's existing constraints", async () => {
    const user = userEvent.setup();
    const onRevise = vi.fn();
    renderReview(
      <ProposalReview
        proposal={{
          ...proposal,
          intent: {
            ...proposal.intent,
            era: "1990s",
            runtimeTargetMin: 180,
            mustInclude: ["Heat"],
          },
        }}
        selfService
        onRevise={onRevise}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Edit brief" }));
    const details = screen.getByText("Other request details (3)").closest("details");
    expect(details).not.toHaveAttribute("open");
    await user.click(screen.getByText("Other request details (3)"));
    expect(details).toHaveTextContent("Era1990s");
    expect(details).toHaveTextContent("Target length180 minutes");
    expect(details).toHaveTextContent("Must includeHeat");

    await user.clear(screen.getByLabelText("Channel brief"));
    await user.type(screen.getByLabelText("Channel brief"), "90s action with more comedy");
    await user.click(screen.getByRole("button", { name: "Update suggestions" }));
    expect(onRevise).toHaveBeenCalledWith({
      description: "90s action with more comedy",
      era: "1990s",
      runtimeTargetMin: 180,
      mustInclude: ["Heat"],
    });
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
    expect(screen.getByText("1 title · 1 will be added")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Create channel" }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("counts a searched library title as ready instead of another acquisition", () => {
    renderReview(
      <ProposalReview
        proposal={proposal}
        status="partially-edited"
        selfService
        edit={{
          add: [
            {
              name: "Point Break",
              mediaType: "movie",
              tmdbId: 1089,
              inLibrary: true,
              libraryItemId: "library-point-break",
            },
          ],
        }}
        onEdit={vi.fn()}
        onApprove={vi.fn()}
      />,
    );

    expect(screen.getByText("3 titles · 2 in your library · 1 will be added")).toBeVisible();
    expect(screen.getByText("Loomarr will add 1 missing title.")).toBeVisible();
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
    const evidence = screen.getByText("Suggestion details").closest("details");
    expect(evidence).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("Suggestion details"));
    expect(evidence).toHaveAttribute("open");
    await userEvent.click(screen.getByText("See the catalog decisions"));
    expect(screen.getByText(/included · matched request, era/)).toBeInTheDocument();
  });

  it("calls out a partial interpretation in plain language", () => {
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
      />,
    );
    const warning = screen.getByText(/Some titles may be a loose match/).closest("p");
    expect(warning).toHaveTextContent("Remove anything that does not fit");
    expect(warning).not.toHaveTextContent("xqz");
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
