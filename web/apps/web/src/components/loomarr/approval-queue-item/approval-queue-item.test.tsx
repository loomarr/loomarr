import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalQueueItem } from "./approval-queue-item";

describe("ApprovalQueueItem", () => {
  it("approves and denies via one click each", () => {
    const onApprove = vi.fn();
    const onDeny = vi.fn();
    render(<ApprovalQueueItem title="90s Action" acquisitions={3} onApprove={onApprove} onDeny={onDeny} />);
    expect(screen.getByText("3 to acquire")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /approve/i }));
    fireEvent.click(screen.getByRole("button", { name: /deny/i }));
    expect(onApprove).toHaveBeenCalledOnce();
    expect(onDeny).toHaveBeenCalledOnce();
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
