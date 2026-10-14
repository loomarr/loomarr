import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TunerLoader } from "./tuner-loader";

// The wash is the dark layer between whatever picture sits under the loader and the snow (#1620 B4).
const wash = (container: HTMLElement) => container.querySelector<HTMLElement>("[data-wash]");

// Every class that animates, across the whole loader. Under reduced motion none may run, so each one
// must be gated `motion-safe:` (jsdom sees the raw class string, not the compiled rule).
const animationClasses = (container: HTMLElement) =>
  Array.from(container.querySelectorAll<HTMLElement>("[class]")).flatMap((el) =>
    Array.from(el.classList).filter((c) => c.includes("animate-")),
  );

describe("TunerLoader", () => {
  it("renders the phosphor bar strip", () => {
    const { container } = render(<TunerLoader />);
    // Seven bars form the level-meter strip (the B4 mock). Counted structurally (the class carries a
    // `motion-safe:` variant prefix that the Tailwind compiler resolves, but jsdom sees the raw
    // string, so a `.animate-signal-lock` selector would miss) — the bars are the spans that carry
    // the signal-400 phosphor colour.
    const bars = container.querySelectorAll("span.bg-signal-400");
    expect(bars).toHaveLength(7);
  });

  it("shows the default TUNING IN readout, and honours a custom label", () => {
    const { getByText, rerender } = render(<TunerLoader />);
    expect(getByText("TUNING IN")).toBeInTheDocument();
    rerender(<TunerLoader label="ACQUIRING SIGNAL" />);
    expect(getByText("ACQUIRING SIGNAL")).toBeInTheDocument();
  });

  it("names the channel being tuned between the bars and the readout", () => {
    const { getByText } = render(<TunerLoader channel={{ number: 12, name: "Saturday Cartoons" }} />);
    expect(getByText("CH 12")).toBeInTheDocument();
    expect(getByText("Saturday Cartoons")).toBeInTheDocument();
  });

  it("is decorative — hidden from the accessibility tree", () => {
    const { container } = render(<TunerLoader />);
    // Motion only; the accessible 'loading' news is carried by the player's status text, not here.
    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
  });

  it("drains a held frame into the snow on a switch", () => {
    const { container } = render(<TunerLoader heldFrame />);
    expect(wash(container)).toHaveAttribute("data-wash", "draining");
    expect(wash(container)?.className).toContain("motion-safe:animate-held-drain");
    // …while the snow thickens in over it; a cold start shows the snow at once.
    const snow = () => container.querySelector<HTMLElement>("[data-snow='wash']");
    expect(snow()?.className).toContain("motion-safe:animate-wash-thicken");
  });

  it("shows the snow at once on a cold start", () => {
    const { container } = render(<TunerLoader />);
    expect(container.querySelector("[data-snow='wash']")?.className).not.toContain("animate-wash-thicken");
  });

  it("sits the wash at rest on a cold start, with nothing to drain", () => {
    const { container } = render(<TunerLoader />);
    expect(wash(container)).toHaveAttribute("data-wash", "rest");
    expect(wash(container)?.className).not.toContain("animate-");
  });

  it("stops every motion under reduced motion, but keeps the still snow", () => {
    const { container } = render(
      <TunerLoader heldFrame channel={{ number: 12, name: "Saturday Cartoons" }} />,
    );
    const classes = animationClasses(container);
    expect(classes.length).toBeGreaterThan(0);
    expect(classes.filter((c) => !c.startsWith("motion-safe:"))).toEqual([]);
    // The frame is replaced by still snow, not by nothing: the wash snow is not hidden without motion.
    const snow = container.querySelector<HTMLElement>("[data-snow='wash']");
    expect(snow).not.toBeNull();
    expect(snow?.classList.contains("hidden")).toBe(false);
  });
});
