import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Input } from "./input";

describe("Input", () => {
  it("forwards value and placeholder", () => {
    render(<Input placeholder="Search…" defaultValue="matrix" aria-label="search" />);
    const el = screen.getByLabelText("search") as HTMLInputElement;
    expect(el).toHaveAttribute("placeholder", "Search…");
    expect(el.value).toBe("matrix");
  });

  it("does not render a reveal toggle for a non-password field", () => {
    render(<Input aria-label="search" />);
    expect(screen.queryByRole("button", { name: /password/i })).not.toBeInTheDocument();
  });

  it("toggles a password field between masked and revealed via the eye button", async () => {
    render(<Input type="password" aria-label="password" defaultValue="hunter2" />);
    const field = screen.getByLabelText("password") as HTMLInputElement;
    // Masked by default.
    expect(field).toHaveAttribute("type", "password");
    // The toggle is keyboard-reachable and labeled for its current action.
    const toggle = screen.getByRole("button", { name: /show password/i });
    await userEvent.click(toggle);
    // Revealed → type flips to text, and the label flips to "Hide password".
    expect(field).toHaveAttribute("type", "text");
    expect(screen.getByRole("button", { name: /hide password/i })).toBeInTheDocument();
    // The value survives the toggle (same underlying input, not remounted).
    expect(field.value).toBe("hunter2");
  });
});
