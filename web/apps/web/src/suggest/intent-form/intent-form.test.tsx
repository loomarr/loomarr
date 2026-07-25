import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { IntentForm } from "./intent-form";

// The form's own comment claims the constraints "now actually reach the server". That
// was true of the four it rendered and silently untrue of mustInclude/mustExclude,
// which the shared schema declared and the scorer consumed (score.go weights a
// must-include match) with no way to set them short of hand-crafting an API call.
//
// These assert on the SUBMITTED INTENT rather than on the inputs, because the defect
// was never that a field looked wrong — it was that a field never travelled.

const openConstraints = () => fireEvent.click(screen.getByRole("button", { name: /add constraints/i }));

describe("IntentForm", () => {
  it("submits a description on its own", () => {
    const onSubmit = vi.fn();
    render(<IntentForm onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "90s action movies" } });
    fireEvent.click(screen.getByRole("button", { name: /suggest/i }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ description: "90s action movies" }));
  });

  it("carries mustInclude and mustExclude to the server", () => {
    const onSubmit = vi.fn();
    render(<IntentForm onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "90s action movies" } });
    openConstraints();
    fireEvent.change(screen.getByLabelText(/must include/i), { target: { value: "Point Break, Heat" } });
    fireEvent.change(screen.getByLabelText(/must exclude/i), { target: { value: "Con Air" } });
    fireEvent.click(screen.getByRole("button", { name: /suggest/i }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        mustInclude: ["Point Break", "Heat"],
        mustExclude: ["Con Air"],
      }),
    );
  });

  it("omits the term lists entirely when left blank", () => {
    // Absence, not []. An empty array is a claim ("nothing is required"); omitting the
    // field is the truth ("the user didn't say"), and omitempty drops it on the wire.
    const onSubmit = vi.fn();
    render(<IntentForm onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "90s action movies" } });
    openConstraints();
    fireEvent.click(screen.getByRole("button", { name: /suggest/i }));

    const intent = onSubmit.mock.calls[0][0];
    expect(intent.mustInclude).toBeUndefined();
    expect(intent.mustExclude).toBeUndefined();
  });

  it("ignores stray separators rather than sending empty terms", () => {
    const onSubmit = vi.fn();
    render(<IntentForm onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "90s action movies" } });
    openConstraints();
    fireEvent.change(screen.getByLabelText(/must include/i), { target: { value: " Heat , , ,Predator " } });
    fireEvent.click(screen.getByRole("button", { name: /suggest/i }));

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ mustInclude: ["Heat", "Predator"] }));
  });

  it("still carries the four original constraints", () => {
    // Guard against the new fields displacing what already worked.
    const onSubmit = vi.fn();
    render(<IntentForm onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "90s action movies" } });
    openConstraints();
    fireEvent.change(screen.getByLabelText(/^era$/i), { target: { value: "1990s" } });
    fireEvent.change(screen.getByLabelText(/^tone$/i), { target: { value: "high-energy" } });
    fireEvent.change(screen.getByLabelText(/target runtime/i), { target: { value: "180" } });
    fireEvent.change(screen.getByLabelText(/max titles/i), { target: { value: "10" } });
    fireEvent.click(screen.getByRole("button", { name: /suggest/i }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        era: "1990s",
        tone: "high-energy",
        runtimeTargetMin: 180,
        maxAcquisitions: 10,
      }),
    );
  });
});
