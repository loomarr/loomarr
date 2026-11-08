import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ArtworkFallback } from "./artwork-fallback";

describe("ArtworkFallback", () => {
  it("is a tinted wash, never hatched", () => {
    render(<ArtworkFallback label="Night Train" />);
    const el = screen.getByRole("presentation");
    expect(el.style.backgroundImage).toContain("linear-gradient");
    expect(el.style.backgroundImage).not.toContain("repeating");
  });

  it("is never blank for a title with no channel: it sets the title's letters", () => {
    render(<ArtworkFallback label="Night Train" />);
    expect(screen.getByText("NT")).toBeInTheDocument();
  });

  it("shows the channel's monogram, tinted by its identity hue", () => {
    render(<ArtworkFallback channel={{ name: "Field Notes", number: 7 }} />);
    expect(screen.getByText("FN").style.color).toBe("var(--color-signal)");
  });

  it("derives a different hue for a different channel number", () => {
    render(<ArtworkFallback channel={{ name: "Night Owl", number: 1 }} />);
    expect(screen.getByText("NO").style.color).toBe("var(--color-suggest)");
  });

  it("prefers the channel's letters over a label", () => {
    render(<ArtworkFallback channel={{ name: "Night Owl", number: 1 }} label="Other Thing" />);
    expect(screen.queryByText("OT")).not.toBeInTheDocument();
  });

  it("merges a caller className with its own layout classes", () => {
    render(<ArtworkFallback className="rounded-md" />);
    const el = screen.getByRole("presentation");
    expect(el.className).toContain("rounded-md");
    expect(el.className).toContain("size-full");
  });
});
