import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FullscreenButton } from "./fullscreen-button";

describe("FullscreenButton", () => {
  it("names itself for the current state", () => {
    const { rerender } = render(<FullscreenButton active={false} onToggle={() => {}} />);
    expect(screen.getByRole("button", { name: "Fullscreen" })).toBeInTheDocument();
    rerender(<FullscreenButton active onToggle={() => {}} />);
    expect(screen.getByRole("button", { name: "Exit fullscreen" })).toBeInTheDocument();
  });

  it("calls onToggle when clicked", async () => {
    const onToggle = vi.fn();
    render(<FullscreenButton active={false} onToggle={onToggle} />);
    await userEvent.click(screen.getByRole("button", { name: "Fullscreen" }));
    expect(onToggle).toHaveBeenCalledOnce();
  });
});
