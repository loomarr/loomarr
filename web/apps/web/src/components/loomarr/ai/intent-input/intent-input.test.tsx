import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { IntentInput } from "./intent-input";

describe("IntentInput", () => {
  it("passes empty input to the owning form for validation", () => {
    const onSubmit = vi.fn();
    render(<IntentInput value="" onValueChange={() => {}} onSubmit={onSubmit} />);

    fireEvent.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it("submits on Ctrl or Command with Enter, and a bare Enter only adds a line", () => {
    const onSubmit = vi.fn();
    render(<IntentInput value="90s action" onValueChange={() => {}} onSubmit={onSubmit} />);
    const box = screen.getByRole("textbox", { name: "Channel intent" });

    fireEvent.keyDown(box, { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();

    fireEvent.keyDown(box, { ctrlKey: true, key: "Enter" });
    expect(onSubmit).toHaveBeenCalledTimes(1);

    fireEvent.keyDown(box, { key: "Enter", metaKey: true });
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("reports what is typed, and holds the text area still while a suggestion is made", () => {
    const onValueChange = vi.fn();
    const { rerender } = render(<IntentInput value="" onValueChange={onValueChange} />);

    fireEvent.change(screen.getByRole("textbox", { name: "Channel intent" }), {
      target: { value: "cozy mysteries" },
    });
    expect(onValueChange).toHaveBeenCalledWith("cozy mysteries");

    rerender(<IntentInput value="cozy mysteries" onValueChange={onValueChange} submitting />);
    expect(screen.getByRole("textbox", { name: "Channel intent" })).toBeDisabled();
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
