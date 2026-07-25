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
});
