import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GenerationProgress } from "./generation-progress";

describe("GenerationProgress", () => {
  it.each([
    ["reasoning", "Understanding your channel…"],
    ["searching", "Looking for matching titles…"],
    ["scoring", "Preparing your channel…"],
  ] as const)("shows one calm status for %s", (phase, label) => {
    const { container } = render(<GenerationProgress phase={phase} round={4} elapsedSeconds={3} />);
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(container.querySelectorAll(".animate-spin")).toHaveLength(1);
    expect(screen.queryByText(/pass|\d+s/i)).not.toBeInTheDocument();
  });

  it("adds reassuring copy for a longer wait without showing a timer", () => {
    render(<GenerationProgress phase="reasoning" elapsedSeconds={12} />);
    expect(screen.getByText(/can take a little while/i)).toBeInTheDocument();
    expect(screen.queryByText("12s")).not.toBeInTheDocument();
  });

  it("leaves a terminal failure to the recovery surface", () => {
    const { container } = render(<GenerationProgress phase="failed" error="Generation failed" />);
    expect(container).toBeEmptyDOMElement();
  });
});
