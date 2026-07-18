import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntentInput } from "./intent-input";

describe("IntentInput", () => {
  it("disables submit until the intent is describable", () => {
    const { rerender } = render(<IntentInput value="" onValueChange={() => {}} />);
    expect(screen.getByRole("button", { name: /suggest a lineup/i })).toBeDisabled();
    rerender(<IntentInput value="90s action" onValueChange={() => {}} />);
    expect(screen.getByRole("button", { name: /suggest a lineup/i })).toBeEnabled();
  });

  it("fills the intent from a template chip", () => {
    const onValueChange = vi.fn();
    render(
      <IntentInput
        value=""
        onValueChange={onValueChange}
        templates={[{ label: "Cozy mysteries", value: "cozy sunday mysteries" }]}
      />,
    );
    fireEvent.click(screen.getByText("Cozy mysteries"));
    expect(onValueChange).toHaveBeenCalledWith("cozy sunday mysteries");
  });

  it("shows a submitting state and blocks submit", () => {
    render(<IntentInput value="90s action" onValueChange={() => {}} submitting />);
    expect(screen.getByText("Suggesting…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /suggesting/i })).toBeDisabled();
  });
});
