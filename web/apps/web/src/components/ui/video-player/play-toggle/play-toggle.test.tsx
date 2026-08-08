import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PlayToggle } from "./play-toggle";

describe("PlayToggle", () => {
  it("names itself Play when paused and Pause when playing", () => {
    const { rerender } = render(<PlayToggle playing={false} onToggle={() => {}} />);
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    rerender(<PlayToggle playing onToggle={() => {}} />);
    expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument();
  });

  it("calls onToggle when clicked", async () => {
    const onToggle = vi.fn();
    render(<PlayToggle playing={false} onToggle={onToggle} />);
    await userEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(onToggle).toHaveBeenCalledOnce();
  });
});
