import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelNav } from "./channel-nav";

const SECTIONS = [
  { id: "info", label: "Channel info" },
  { id: "lineup", label: "Lineup" },
  { id: "filler", label: "Filler" },
];

describe("ChannelNav", () => {
  it("lists the section labels", () => {
    render(<ChannelNav sections={SECTIONS} activeId="info" onSelect={() => {}} />);
    for (const s of SECTIONS) expect(screen.getByRole("button", { name: s.label })).toBeInTheDocument();
  });

  it("marks the active section with aria-current", () => {
    render(<ChannelNav sections={SECTIONS} activeId="filler" onSelect={() => {}} />);
    expect(screen.getByRole("button", { name: "Filler" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: "Lineup" })).not.toHaveAttribute("aria-current");
  });

  it("calls onSelect with the clicked section id", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<ChannelNav sections={SECTIONS} activeId="info" onSelect={onSelect} />);
    await user.click(screen.getByRole("button", { name: "Lineup" }));
    expect(onSelect).toHaveBeenCalledWith("lineup");
  });
});
