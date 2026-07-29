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

  // The tool loop alternates thinking and searching, so a step that is about to run
  // again must NOT be ticked off. Marking reasoning done the moment a search starts
  // would show a checkmark that un-ticks on the next pass.
  it("does not complete an earlier step while the tool loop is still running", () => {
    const { container } = render(<GenerationProgress phase="searching" round={2} />);
    expect(screen.getByText("Searching the library…")).toBeInTheDocument();
    // Assert the tick MARK, not the label: a completed step still renders its text, so
    // checking for "Pick titles that fit" would pass whether or not it was ticked off.
    // Reasoning runs again after this search, so nothing may be marked done yet.
    expect(container.querySelectorAll(".text-lock")).toHaveLength(0);
  });

  // Scoring runs outside the loop, so reaching it genuinely means the loop is over.
  it("completes the loop steps once the run reaches scoring", () => {
    const { container } = render(<GenerationProgress phase="scoring" />);
    // Two ticks: the two loop steps are behind us for real.
    expect(container.querySelectorAll(".text-lock")).toHaveLength(2);
  });

  // A short run should not be narrated; the count only appears once the wait is long
  // enough that silence would read as broken.
  it("hides elapsed time below the threshold and shows it above", () => {
    const { rerender } = render(<GenerationProgress phase="reasoning" elapsedSeconds={1} />);
    expect(screen.queryByText(/\d+s/)).not.toBeInTheDocument();

    rerender(<GenerationProgress phase="reasoning" elapsedSeconds={12} />);
    expect(screen.getByText("12s")).toBeInTheDocument();
  });

  // Pass 1 is the ordinary case and adds nothing; from pass 2 the number is what tells
  // the viewer that repetition is the model working rather than the UI being stuck.
  it("names the pass only once the loop has repeated", () => {
    const { rerender } = render(<GenerationProgress phase="reasoning" round={1} elapsedSeconds={9} />);
    expect(screen.getByText("9s")).toBeInTheDocument();
    expect(screen.queryByText(/pass/)).not.toBeInTheDocument();

    rerender(<GenerationProgress phase="reasoning" round={3} elapsedSeconds={9} />);
    expect(screen.getByText("pass 3 · 9s")).toBeInTheDocument();
  });
});
