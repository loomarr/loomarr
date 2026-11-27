import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { OnAirIndicator } from "./on-air-indicator";

describe("OnAirIndicator", () => {
  it("always announces its status to screen readers", () => {
    render(<OnAirIndicator state="live" />);
    expect(screen.getByRole("img", { name: "On air" })).toBeInTheDocument();
  });

  it("shows the mono label only when asked, and still says the status once", () => {
    const { rerender } = render(<OnAirIndicator state="reconciling" />);
    expect(screen.getByRole("img", { name: "Updating" })).toBeInTheDocument();
    expect(screen.queryByText("Updating")).not.toBeInTheDocument();
    rerender(<OnAirIndicator state="reconciling" showLabel />);
    expect(screen.getByText("Updating")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Updating" })).not.toBeInTheDocument();
  });
});
