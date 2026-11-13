import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { ColorBars } from "./color-bars";

describe("ColorBars", () => {
  it("renders the seven test-card segments", () => {
    const { container } = render(<ColorBars />);
    const strips = [...container.querySelectorAll("[aria-hidden]")].filter((el) => el.children.length === 7);
    expect(strips).toHaveLength(1);
  });

  it("is decorative — hidden from the accessibility tree", () => {
    const { container } = render(<ColorBars />);
    // It names nothing; screen readers must skip it (§1: nostalgia in the margins).
    const strip = [...container.querySelectorAll("[aria-hidden]")].find((el) => el.children.length === 7);
    expect(strip).toHaveAttribute("aria-hidden", "true");
  });
});
