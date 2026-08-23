import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalQueueItem } from "./approval-queue-item";

describe("ApprovalQueueItem", () => {
  it("approves in one click; deny arms a reason first", () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    render(<ApprovalQueueItem title="90s Action" acquisitions={3} onApprove={onApprove} onDeny={onDeny} />);
    expect(screen.getByText("3 to acquire")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /approve/i }));
    expect(onApprove).toHaveBeenCalledOnce();

    // Deny is deliberately two-step: approving needs no explanation, declining does.
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onDeny).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onDeny).toHaveBeenCalledOnce();
  });

  it("sends the typed reason to onDeny", () => {
    // A1: the API has persisted `denyReason` and this component has rendered it since
    // day one, but every call site sent `{}` — so the field was always empty and a
    // member never learned why. This is the missing capture half.
    const onDeny = vi.fn();
    render(<ApprovalQueueItem title="X" onDeny={onDeny} />);
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    fireEvent.change(screen.getByLabelText(/why not/i), {
      target: { value: "Over the cap this week." },
    });
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onDeny).toHaveBeenCalledWith("Over the cap this week.");
  });

  it("sends undefined rather than an empty string when no reason is typed", () => {
    // The reason is optional by design — requiring one would make declining a chore.
    // But "" must not reach the API as a reason: omitempty drops it, and a blank
    // string would render as an empty explanation rather than none at all.
    const onDeny = vi.fn();
    render(<ApprovalQueueItem title="X" onDeny={onDeny} />);
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onDeny).toHaveBeenCalledWith(undefined);
  });

  it("cancel abandons the deny without calling onDeny", () => {
    const onDeny = vi.fn();
    render(<ApprovalQueueItem title="X" onDeny={onDeny} />);
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onDeny).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /approve/i })).toBeInTheDocument();
  });

  it("disables actions while approving", () => {
    render(<ApprovalQueueItem title="X" status="approving" />);
    expect(screen.getByRole("button", { name: /approve/i })).toBeDisabled();
  });

  it("shows the deny reason and retires the actions when denied", () => {
    render(<ApprovalQueueItem title="X" status="denied" denyReason="Over the acquisition cap." />);
    expect(screen.getByText("Over the acquisition cap.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
  });

  // --- §4 PROPOSAL HONESTY (#259) -------------------------------------------------------------

  // A refusal changes what the Approve button MEANS, so it renders on the card itself rather than
  // behind the "Show picks" toggle. Before this, an admin approved seven titles, got five, and
  // nothing on the card said which two or why.
  it("names refused titles and their rating WITHOUT expanding the picks toggle", () => {
    render(
      <ApprovalQueueItem
        title="90s Saturday morning cartoons"
        lineup={[
          { mediaType: "movie", name: "Sunny Toons", tmdbId: 5001, officialRating: "TV-Y7", inLibrary: true },
        ]}
        refused={[
          {
            item: {
              mediaType: "movie",
              name: "Midnight Toons",
              tmdbId: 5004,
              officialRating: "TV-MA",
              inLibrary: true,
            },
            reason: "over_ceiling",
          },
        ]}
      />,
    );

    // ⚠ No click first. The whole point is that this is visible before any interaction.
    expect(screen.getByText(/1 title won't be included/i)).toBeInTheDocument();
    expect(screen.getByText("Midnight Toons")).toBeInTheDocument();
    expect(screen.getByText(/rated TV-MA, above this channel's audience limit/i)).toBeInTheDocument();
    // The picks list is still collapsed, so the refusal is not merely leaking out of it.
    expect(screen.queryByText("Sunny Toons")).not.toBeInTheDocument();
  });

  // Nothing refused ⇒ nothing rendered. A permanent "0 titles won't be included" would train an
  // admin to skip the notice on exactly the rows where it matters.
  it("renders no refusal notice when nothing was refused", () => {
    render(<ApprovalQueueItem title="80s action heroes" refused={[]} />);
    expect(screen.queryByText(/won't be included/i)).not.toBeInTheDocument();
  });

  it("records explicit taste separately from approving or dropping a pick", () => {
    const onFeedback = vi.fn();
    render(
      <ApprovalQueueItem
        title="Classic Simpsons"
        lineup={[{ mediaType: "series", name: "The Simpsons", tmdbId: 456, inLibrary: true }]}
        onFeedback={onFeedback}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /show picks/i }));
    fireEvent.click(screen.getByRole("button", { name: "Never" }));
    expect(onFeedback).toHaveBeenCalledWith(expect.objectContaining({ tmdbId: 456 }), "never");
  });
});
