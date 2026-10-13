import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The snow tile is cached per page (module state), so every test imports a fresh TvStatic.
const freshTvStatic = async () => {
  vi.resetModules();
  return (await import("./tv-static")).TvStatic;
};

// jsdom cannot decode an image or draw a canvas. Decoding resolves here, and a missing 2D context
// takes the tile's no-bitmap path: the SVG itself as the background.
const decode = vi.fn(() => Promise.resolve());
const reducedMotion = (reduce: boolean) =>
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: reduce && query.includes("reduce") }));

beforeEach(() => {
  Object.defineProperty(HTMLImageElement.prototype, "decode", { configurable: true, value: decode });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
  reducedMotion(false);
});

afterEach(() => {
  Reflect.deleteProperty(HTMLImageElement.prototype, "decode");
  decode.mockClear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const snowLayer = (container: HTMLElement) =>
  container.querySelector<HTMLElement>("[style*='background-image'][style*='data:image/svg']");

describe("TvStatic", () => {
  it("is inert — hidden from a11y and non-interactive", async () => {
    const TvStatic = await freshTvStatic();
    const { container } = render(<TvStatic />);
    const layer = container.firstElementChild as HTMLElement;
    expect(layer).toHaveAttribute("aria-hidden", "true");
    expect(layer.className).toContain("pointer-events-none");
  });

  it("is off unless motion is allowed (reduced-motion + visual-test gate)", async () => {
    const TvStatic = await freshTvStatic();
    const { container } = render(<TvStatic />);
    const layer = container.firstElementChild as HTMLElement;
    // Hidden by default; only `motion-safe:` re-shows it — so reduced-motion (and the
    // visual suite, which pins it) never render the noise.
    expect(layer.className).toContain("hidden");
    expect(layer.className).toContain("motion-safe:block");
  });

  it("never renders the snow behind idle surfaces under reduced motion, where it is not shown", async () => {
    reducedMotion(true);
    const TvStatic = await freshTvStatic();
    const { container } = render(<TvStatic />);
    // Two frames' worth of waiting: the tile would have been requested by now.
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(decode).not.toHaveBeenCalled();
    expect(container.firstElementChild).toHaveAttribute("data-snow-pending");
  });

  it("paints the page first, then fades the snow in from one pre-rendered tile", async () => {
    const TvStatic = await freshTvStatic();
    const { container } = render(<TvStatic />);
    // First render: no noise to raster yet, so the content in front of it paints at once.
    expect(snowLayer(container)).toBeNull();
    expect(container.firstElementChild).toHaveAttribute("data-snow-pending");
    expect(container.querySelector<HTMLElement>("[style*='opacity: 0']")).not.toBeNull();

    await waitFor(() => expect(snowLayer(container)).not.toBeNull());
    const snow = snowLayer(container) as HTMLElement;
    // Streaks over grain, as two repeating tiles; the existing stepped flicker moves them.
    expect(snow.style.backgroundImage.match(/url\("/g)).toHaveLength(2);
    expect(snow.style.backgroundSize).toBe("1376px 344px, 688px 344px");
    expect(snow.className).toContain("motion-safe:animate-tv-snow");
    // It fades up to its own (class) opacity rather than popping in.
    expect(snow.style.opacity).toBe("");
    expect(snow.style.transition).toContain("opacity");
    expect(container.firstElementChild).not.toHaveAttribute("data-snow-pending");
  });

  it("draws the noise once per page, however many surfaces show it", async () => {
    const TvStatic = await freshTvStatic();
    const first = render(<TvStatic />);
    await waitFor(() => expect(snowLayer(first.container)).not.toBeNull());
    const calls = decode.mock.calls.length;
    // A later surface (the tuner's wash) has the tile from its first render.
    const { container } = render(<TvStatic variant="wash" />);
    expect(snowLayer(container)).not.toBeNull();
    expect(decode.mock.calls.length).toBe(calls);
  });

  it("keeps the switch wash as still snow under reduced motion; only its flicker is gated", async () => {
    reducedMotion(true);
    const TvStatic = await freshTvStatic();
    const { container } = render(<TvStatic variant="wash" />);
    const layer = container.firstElementChild as HTMLElement;
    expect(layer).toHaveAttribute("aria-hidden", "true");
    expect(layer.classList.contains("hidden")).toBe(false);
    await waitFor(() => expect(snowLayer(container)?.className).toContain("motion-safe:animate-tv-snow"));
  });
});
