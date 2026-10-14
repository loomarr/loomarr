import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntentInput } from "./intent-input";

describe("IntentInput", () => {
  it("passes empty input to the owning form for validation", () => {
    const onSubmit = vi.fn();
    render(<IntentInput value="" onValueChange={() => {}} onSubmit={onSubmit} />);

    fireEvent.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(onSubmit).toHaveBeenCalledOnce();
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
    const onSubmit = vi.fn();
    render(<IntentInput value="90s action" onValueChange={() => {}} onSubmit={onSubmit} submitting />);
    expect(screen.getByText("Suggesting…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /suggesting/i })).toBeDisabled();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
