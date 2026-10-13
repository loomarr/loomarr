import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalGroup } from "./approval-group";
import type { ApprovalGroupRow } from "./approval-group.type";

const row = (id: string, label: string, excluded = false): ApprovalGroupRow => ({
  id,
  label,
  excluded,
  content: <p>{label}</p>,
});

const Harness = ({ rows, onApprove = () => {} }: { rows: ApprovalGroupRow[]; onApprove?: () => void }) => {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  return (
    <ApprovalGroup
      title="Channel requests"
      rows={rows}
      selected={selected}
      onSelectedChange={setSelected}
      onApprove={onApprove}
    />
  );
};

describe("ApprovalGroup", () => {
  it("is a named group whose Approve counts the selection and is disabled at zero", async () => {
    const onApprove = vi.fn();
    render(<Harness rows={[row("a", "Noir"), row("b", "Westerns")]} onApprove={onApprove} />);

    const group = screen.getByRole("group", { name: "Channel requests" });
    expect(within(group).getByText("None selected")).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: "Approve" })).toBeDisabled();

    await userEvent.click(within(group).getByRole("checkbox", { name: "Select Noir for bulk approve" }));
    expect(within(group).getByText("1 selected")).toBeInTheDocument();
    await userEvent.click(within(group).getByRole("button", { name: "Approve 1" }));
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it("selects and clears every bulkable row from the keyboard", async () => {
    render(<Harness rows={[row("a", "Noir"), row("b", "Westerns")]} />);

    const selectAll = screen.getByRole("checkbox", { name: "Select all Channel requests" });
    selectAll.focus();
    await userEvent.keyboard(" ");
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    await userEvent.keyboard(" ");
    expect(screen.getByText("None selected")).toBeInTheDocument();
  });

  it("gives an excluded row no checkbox and says why, and select-all skips it", async () => {
    render(<Harness rows={[row("a", "Noir"), row("b", "Westerns"), row("c", "Edited one", true)]} />);

    expect(
      screen.queryByRole("checkbox", { name: "Select Edited one for bulk approve" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("Excluded from bulk — approve individually so the edit is kept."),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("checkbox", { name: "Select all Channel requests" }));
    expect(screen.getByRole("button", { name: "Approve 2" })).toBeEnabled();
  });

  it("offers no select-all over a single bulkable row", () => {
    render(<Harness rows={[row("a", "Noir"), row("c", "Edited one", true)]} />);

    expect(screen.queryByRole("checkbox", { name: "Select all Channel requests" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Select Noir for bulk approve" })).toBeInTheDocument();
  });
});
