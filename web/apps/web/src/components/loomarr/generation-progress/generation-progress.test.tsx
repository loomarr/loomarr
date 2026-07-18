import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GenerationProgress } from "./generation-progress";

describe("GenerationProgress", () => {
  it("marks the current step active and shows its in-flight label", () => {
    render(<GenerationProgress phase="scoring" />);
    expect(screen.getByText("Scoring the lineup…")).toBeInTheDocument();
    expect(screen.getByText("Search the library")).toBeInTheDocument();
  });

  it("renders the problem text on failure, never a raw stack", () => {
    render(<GenerationProgress phase="failed" error="The model timed out." />);
    expect(screen.getByRole("alert")).toHaveTextContent("The model timed out.");
  });
});
