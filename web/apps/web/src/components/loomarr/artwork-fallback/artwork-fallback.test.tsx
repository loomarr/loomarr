import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ArtworkFallback } from "./artwork-fallback";

describe("ArtworkFallback", () => {
  it("renders the line motif with no monogram when no channel is known", () => {
    render(<ArtworkFallback />);
    const el = screen.getByRole("presentation");
    expect(el).toBeInTheDocument();
    expect(el.textContent).toBe("");
  });

  it("shows the channel's monogram, tinted by its identity hue, when a channel is given", () => {
    render(<ArtworkFallback channel={{ name: "Field Notes", number: 7 }} />);
    const mark = screen.getByText("FN");
    expect(mark).toBeInTheDocument();
    expect(mark.style.color).toBe("var(--color-signal)");
  });

  it("derives a different hue for a different channel number", () => {
    render(<ArtworkFallback channel={{ name: "Night Owl", number: 1 }} />);
    expect(screen.getByText("NO").style.color).toBe("var(--color-suggest)");
  });

  it("merges a caller className with its own layout classes", () => {
    render(<ArtworkFallback className="rounded-md" />);
    const el = screen.getByRole("presentation");
    expect(el.className).toContain("rounded-md");
    expect(el.className).toContain("size-full");
  });
});
