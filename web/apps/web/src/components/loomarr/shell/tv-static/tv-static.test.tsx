import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TvStatic } from "./tv-static";

describe("TvStatic", () => {
  it("is inert — hidden from a11y and non-interactive", () => {
    const { container } = render(<TvStatic />);
    const layer = container.firstElementChild as HTMLElement;
    expect(layer).toHaveAttribute("aria-hidden", "true");
    expect(layer.className).toContain("pointer-events-none");
  });

  it("is off unless motion is allowed (reduced-motion + visual-test gate)", () => {
    const { container } = render(<TvStatic />);
    const layer = container.firstElementChild as HTMLElement;
    // Hidden by default; only `motion-safe:` re-shows it — so reduced-motion (and the
    // visual suite, which pins it) never render the noise.
    expect(layer.className).toContain("hidden");
    expect(layer.className).toContain("motion-safe:block");
  });
});
