import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelDangerZone } from "./channel-danger-zone";

const base = {
  channelName: "90s Action",
  status: "live",
  onPause: vi.fn(),
  onResume: vi.fn(),
  onDelete: vi.fn(),
};

describe("ChannelDangerZone", () => {
  it("shows Pause with the off-air copy for a non-paused channel", () => {
    render(<ChannelDangerZone {...base} />);
    expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Resume" })).not.toBeInTheDocument();
    expect(screen.getByText("Take this channel off air without deleting it.")).toBeInTheDocument();
  });

  it("shows Resume with the paused copy when status is paused", () => {
    render(<ChannelDangerZone {...base} status="paused" />);
    expect(screen.getByRole("button", { name: "Resume" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Pause" })).not.toBeInTheDocument();
    expect(screen.getByText("Paused: off air but kept.")).toBeInTheDocument();
  });

  it("calls onPause / onResume from the respective control", async () => {
    const onPause = vi.fn();
    render(<ChannelDangerZone {...base} onPause={onPause} />);
    await userEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(onPause).toHaveBeenCalledTimes(1);

    const onResume = vi.fn();
    render(<ChannelDangerZone {...base} status="paused" onResume={onResume} />);
    await userEvent.click(screen.getByRole("button", { name: "Resume" }));
    expect(onResume).toHaveBeenCalledTimes(1);
  });

  it("hides the confirm step until Delete channel is clicked", () => {
    render(<ChannelDangerZone {...base} />);
    expect(screen.queryByRole("button", { name: "Delete permanently" })).not.toBeInTheDocument();
  });

  it("reveals a two-step confirm (no name typing) — arm, then execute", async () => {
    render(<ChannelDangerZone {...base} />);
    // Step 1: arm.
    await userEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    // Step 2 appears, ready to execute — no name to type, no textbox to fill.
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete permanently" })).toBeEnabled();
  });

  it("calls onDelete with purge:false by default", async () => {
    const onDelete = vi.fn();
    render(<ChannelDangerZone {...base} onDelete={onDelete} />);

    await userEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    await userEvent.click(screen.getByRole("button", { name: "Delete permanently" }));

    expect(onDelete).toHaveBeenCalledWith({ purge: false });
  });

  it("calls onDelete with purge:true when the Tunarr checkbox is checked", async () => {
    const onDelete = vi.fn();
    render(<ChannelDangerZone {...base} onDelete={onDelete} />);

    await userEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    await userEvent.click(screen.getByLabelText("Also remove it from Tunarr"));
    await userEvent.click(screen.getByRole("button", { name: "Delete permanently" }));

    expect(onDelete).toHaveBeenCalledWith({ purge: true });
  });

  it("cancel collapses the confirm step and resets the purge choice", async () => {
    render(<ChannelDangerZone {...base} />);
    await userEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    await userEvent.click(screen.getByLabelText("Also remove it from Tunarr"));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("button", { name: "Delete permanently" })).not.toBeInTheDocument();

    // Reopening starts clean — the purge checkbox is unchecked again.
    await userEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    expect(screen.getByLabelText("Also remove it from Tunarr")).not.toBeChecked();
  });

  it("disables every control while busy", () => {
    render(<ChannelDangerZone {...base} busy />);
    expect(screen.getByRole("button", { name: "Pause" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete channel" })).toBeDisabled();
  });
});
