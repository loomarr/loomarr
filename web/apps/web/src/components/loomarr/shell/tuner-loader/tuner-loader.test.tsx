import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TunerLoader } from "./tuner-loader";

describe("TunerLoader", () => {
  it("renders the phosphor bar strip", () => {
    const { container } = render(<TunerLoader />);
    // Nine bars form the level-meter strip. Counted structurally (the class carries a
    // `motion-safe:` variant prefix that the Tailwind compiler resolves, but jsdom sees the raw
    // string, so a `.animate-signal-lock` selector would miss) — the bars are the spans that carry
    // the signal-400 phosphor colour.
    const bars = container.querySelectorAll("span.bg-signal-400");
    expect(bars).toHaveLength(9);
  });

  it("shows the default TUNING IN readout, and honours a custom label", () => {
    const { getByText, rerender } = render(<TunerLoader />);
    expect(getByText("TUNING IN")).toBeInTheDocument();
    rerender(<TunerLoader label="ACQUIRING SIGNAL" />);
    expect(getByText("ACQUIRING SIGNAL")).toBeInTheDocument();
  });

  it("is decorative — hidden from the accessibility tree", () => {
    const { container } = render(<TunerLoader />);
    // Motion only; the accessible 'loading' news is carried by the player's status text, not here.
    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
  });
});
