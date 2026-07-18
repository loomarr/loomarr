import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OnAirIndicator } from "./on-air-indicator";

describe("OnAirIndicator", () => {
  it("always announces its status to screen readers", () => {
    render(<OnAirIndicator state="live" />);
    expect(screen.getByText("On air")).toBeInTheDocument();
  });

  it("shows the mono label only when asked", () => {
    const { rerender } = render(<OnAirIndicator state="reconciling" />);
    // sr-only text is present but the visible mono label is not.
    expect(screen.getAllByText("Reconciling")).toHaveLength(1);
    rerender(<OnAirIndicator state="reconciling" showLabel />);
    expect(screen.getAllByText("Reconciling")).toHaveLength(2);
  });
});
