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

  it("offers separate stop-managing and permanent-delete actions with honest consequences", () => {
    render(<ChannelDangerZone {...base} />);

    expect(screen.getByRole("button", { name: "Stop managing" })).toBeInTheDocument();
    expect(
      screen.getByText("Loomarr keeps its record and leaves any Tunarr channel in place."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" })).toBeInTheDocument();
    expect(
      screen.getByText("Permanently delete Loomarr's record and any retained Tunarr channel."),
    ).toBeInTheDocument();
  });

  it("confirms stop-managing separately and sends purge:false", async () => {
    const onDelete = vi.fn();
    render(<ChannelDangerZone {...base} onDelete={onDelete} />);

    await userEvent.click(screen.getByRole("button", { name: "Stop managing" }));
    expect(
      screen.getByText(
        "Stop managing 90s Action? Loomarr will keep its record and leave any Tunarr channel in place.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Stop managing" }));

    expect(onDelete).toHaveBeenCalledWith({ purge: false });
  });

  it("confirms permanent deletion separately and sends purge:true", async () => {
    const onDelete = vi.fn();
    render(<ChannelDangerZone {...base} onDelete={onDelete} />);

    await userEvent.click(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" }));
    expect(screen.getByText("Delete 90s Action from Loomarr and Tunarr for good?")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" }));

    expect(onDelete).toHaveBeenCalledWith({ purge: true });
  });

  it("cancel returns to both choices", async () => {
    render(<ChannelDangerZone {...base} />);
    await userEvent.click(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" }));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByRole("button", { name: "Stop managing" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" })).toBeInTheDocument();
  });

  it("disables every control while busy", () => {
    render(<ChannelDangerZone {...base} busy />);
    expect(screen.getByRole("button", { name: "Pause" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Stop managing" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" })).toBeDisabled();
  });
});
