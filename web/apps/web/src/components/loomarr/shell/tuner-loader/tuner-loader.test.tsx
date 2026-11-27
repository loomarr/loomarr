import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { TunerLoader } from "./tuner-loader";

// The wash is the dark layer between whatever picture sits under the loader and the snow (#1620 B4).
const wash = (container: HTMLElement) => container.querySelector<HTMLElement>("[data-wash]");
const snow = (container: HTMLElement) => container.querySelector<HTMLElement>("[data-snow='wash']");

// What moves is named by `data-motion`; the stylesheet animates those only where motion is allowed,
// and tests/visual/motion.spec.ts asserts that in a real browser.
const motions = (container: HTMLElement) =>
  Array.from(container.querySelectorAll<HTMLElement>("[data-motion]"), (el) => el.dataset.motion);

describe("TunerLoader", () => {
  it("renders the phosphor bar strip", () => {
    const { container } = render(<TunerLoader />);
    // Seven bars form the level-meter strip (the B4 mock).
    expect(motions(container).filter((motion) => motion === "signal-lock")).toHaveLength(7);
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
    expect(wash(container)).toHaveAttribute("data-motion", "held-drain");
    // …while the snow thickens in over it; a cold start shows the snow at once.
    expect(snow(container)).toHaveAttribute("data-motion", "wash-thicken");
  });

  it("shows the snow at once on a cold start", () => {
    const { container } = render(<TunerLoader />);
    expect(snow(container)).not.toHaveAttribute("data-motion");
    expect(snow(container)?.querySelector("pattern")).not.toBeNull();
  });

  it("sits the wash at rest on a cold start, with nothing to drain", () => {
    const { container } = render(<TunerLoader />);
    expect(wash(container)).toHaveAttribute("data-wash", "rest");
    expect(wash(container)).not.toHaveAttribute("data-motion");
  });

  it("rests drained, with still snow, under the frame its motion starts from", () => {
    const { container } = render(<TunerLoader heldFrame />);
    // Without the animation the wash is already dark and the bars already locked amber.
    expect(wash(container)?.style.backgroundColor).toContain("static-950) 88%");
    expect(container.querySelector<HTMLElement>("[data-motion='signal-lock']")?.style.height).toBe("64%");
    expect(snow(container)?.querySelector("pattern")).not.toBeNull();
  });
});
