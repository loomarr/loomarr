import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { IdleSnow } from "./idle-snow";

const preference = vi.hoisted(() => ({ reducedMotion: null as boolean | null }));
vi.mock("@loomarr/design-system", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@loomarr/design-system")>()),
  useReducedMotionPreference: () => preference.reducedMotion,
}));

const snow = (container: HTMLElement) => container.querySelector("[aria-hidden] pattern");

describe("IdleSnow", () => {
  it("draws inert snow when motion is allowed", () => {
    preference.reducedMotion = false;
    const { container } = render(<IdleSnow />);
    expect(snow(container)).not.toBeNull();
  });

  it("draws nothing under reduced motion", () => {
    preference.reducedMotion = true;
    const { container } = render(<IdleSnow />);
    expect(snow(container)).toBeNull();
  });

  it("draws nothing until the preference is known", () => {
    preference.reducedMotion = null;
    const { container } = render(<IdleSnow />);
    expect(snow(container)).toBeNull();
  });
});
