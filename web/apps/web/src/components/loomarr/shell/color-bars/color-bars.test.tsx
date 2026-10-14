import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ColorBars } from "./color-bars";

describe("ColorBars", () => {
  it("renders the seven test-card segments", () => {
    const { container } = render(<ColorBars />);
    const strip = container.firstElementChild as HTMLElement;
    expect(strip.children).toHaveLength(7);
  });

  it("is decorative — hidden from the accessibility tree", () => {
    const { container } = render(<ColorBars />);
    // It names nothing; screen readers must skip it (§1: nostalgia in the margins).
    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
  });
});
