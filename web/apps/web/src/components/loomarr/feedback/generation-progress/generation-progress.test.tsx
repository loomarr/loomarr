import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GenerationProgress } from "./generation-progress";

// The active step is the ONE the spinner sits on. The suggester loop alternates
// reasoning↔searching, so those two phases must resolve to the SAME step — otherwise the
// active indicator marches forward then jumps backward on the next pass, a checklist walking
// in reverse. `activeStep` reads which of the two rows carries the spinner.
const activeStep = (container: HTMLElement): string | null =>
  container.querySelector(".animate-spin")?.closest("li")?.textContent?.replace(/….*$/, "") ?? null;

describe("GenerationProgress", () => {
  it("marks the current step active and shows its in-flight label", () => {
    render(<GenerationProgress phase="scoring" />);
    expect(screen.getByText("Scoring the lineup…")).toBeInTheDocument();
    expect(screen.getByText("Find the titles")).toBeInTheDocument();
  });

  it("renders the problem text on failure, never a raw stack", () => {
    render(<GenerationProgress phase="failed" error="The model timed out." />);
    expect(screen.getByRole("alert")).toHaveTextContent("The model timed out.");
  });

  // THE BUG THIS FIXES. The backend emits reasoning → searching → reasoning → … for as many
  // passes as the model needs. Both phases are one piece of work ("find the titles"), so the
  // active spinner must NOT move between them — before the fix it ping-ponged step 1 ↔ step 2
  // on every flip, which read as broken.
  it("keeps the same step active across the reasoning↔searching loop (no backward jump)", () => {
    const { container, rerender } = render(<GenerationProgress phase="reasoning" round={1} />);
    const first = activeStep(container);
    expect(first).toBe("Finding the titles");

    rerender(<GenerationProgress phase="searching" round={1} />);
    expect(activeStep(container)).toBe(first); // searching stays on the same row

    rerender(<GenerationProgress phase="reasoning" round={2} />);
    expect(activeStep(container)).toBe(first); // …and back again, still no movement
    // Nothing is ticked off mid-loop: the run has not moved past finding the titles.
    expect(container.querySelectorAll(".text-lock")).toHaveLength(0);
  });

  // Scoring runs after the loop, so reaching it genuinely means finding is done.
  it("completes the find step once the run reaches scoring", () => {
    const { container } = render(<GenerationProgress phase="scoring" />);
    // One tick: "Find the titles" is behind us for real; "Score the lineup" is active.
    expect(container.querySelectorAll(".text-lock")).toHaveLength(1);
    expect(activeStep(container)).toBe("Scoring the lineup");
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
