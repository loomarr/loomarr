import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Switch } from "./switch";

describe("Switch", () => {
  // ⚠ `role="switch"`, not the implicit checkbox role. A checkbox announces "checked"; a switch
  // announces "on"/"off", which is what this control actually means. It is also what every
  // caller queries by, so the role is part of the contract rather than a detail.
  it("is a switch, and reports its state", () => {
    const { rerender } = render(<Switch checked={false} onChange={() => {}} aria-label="Use /data/filler" />);
    expect(screen.getByRole("switch", { name: "Use /data/filler" })).not.toBeChecked();

    rerender(<Switch checked onChange={() => {}} aria-label="Use /data/filler" />);
    expect(screen.getByRole("switch", { name: "Use /data/filler" })).toBeChecked();
  });

  it("toggles on click", async () => {
    const onChange = vi.fn();
    render(<Switch checked={false} onChange={onChange} aria-label="Use it" />);

    await userEvent.click(screen.getByRole("switch"));
    expect(onChange).toHaveBeenCalled();
  });

  // ⚠ THE reason this is a native input rather than the mock's <button>. Space activates a
  // checkbox for free; on a button it is behaviour we would have to write and could forget.
  it("toggles from the keyboard", async () => {
    const onChange = vi.fn();
    render(<Switch checked={false} onChange={onChange} aria-label="Use it" />);

    await userEvent.tab();
    expect(screen.getByRole("switch")).toHaveFocus();
    await userEvent.keyboard(" ");
    expect(onChange).toHaveBeenCalled();
  });

  // ⚠ The input is `sr-only`, which keeps it focusable — `hidden` or `display:none` would make
  // the control unreachable by keyboard while still looking fine on screen.
  it("stays focusable despite being visually hidden", async () => {
    render(<Switch checked={false} onChange={() => {}} aria-label="Use it" />);
    await userEvent.tab();
    expect(screen.getByRole("switch")).toHaveFocus();
  });

  it("does not toggle when disabled", async () => {
    const onChange = vi.fn();
    render(<Switch checked={false} disabled onChange={onChange} aria-label="Use it" />);

    await userEvent.click(screen.getByRole("switch"));
    expect(onChange).not.toHaveBeenCalled();
  });
});
