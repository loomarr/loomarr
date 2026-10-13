import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SnowTile } from "./snow-tile.type";

// The tile is cached per page (module state), so every test takes a fresh copy of the hook.
const freshHook = async () => {
  vi.resetModules();
  return (await import("./snow-tile")).useSnowTile;
};

// jsdom cannot decode an image or draw a canvas. Decoding resolves here, and a missing 2D context
// takes the no-bitmap path, where the tile's background IS the generated SVG — so these tests read
// the noise the module asks the browser to draw.
const decode = vi.fn(() => Promise.resolve());
const pixelRatio = (ratio: number) =>
  Object.defineProperty(window, "devicePixelRatio", { configurable: true, value: ratio });

beforeEach(() => {
  Object.defineProperty(HTMLImageElement.prototype, "decode", { configurable: true, value: decode });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
  pixelRatio(1);
});

afterEach(() => {
  Reflect.deleteProperty(HTMLImageElement.prototype, "decode");
  Reflect.deleteProperty(window, "devicePixelRatio");
  decode.mockReset();
  decode.mockImplementation(() => Promise.resolve());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// The wash's path: asked for at once, no frame to wait on.
const loadTile = async (): Promise<SnowTile> => {
  const useSnowTile = await freshHook();
  const { result } = renderHook(() => useSnowTile(true, false));
  await waitFor(() => expect(result.current).toBeDefined());
  return result.current as SnowTile;
};

// The two backgrounds' SVG sources, streaks first (they composite over the grain).
const svgs = (tile: SnowTile) =>
  Array.from(tile.backgroundImage.matchAll(/url\("data:image\/svg\+xml,([^"]+)"\)/g), (m) =>
    decodeURIComponent(m[1] as string),
  );

describe("useSnowTile", () => {
  it("generates the same seeded snow on every page", async () => {
    const first = await loadTile();
    const second = await loadTile();
    // Two separate pages, rendered from scratch: byte-identical noise, so a still frame never shifts.
    expect(second).toEqual(first);

    const [streaks, grain] = svgs(first);
    // The approved E1 snow: tall fractal grain on a hard gain, under wide faint streaks, each with
    // a fixed seed and stitched so the repeat has no seam.
    expect(grain).toContain('baseFrequency="1.6 2.8" numOctaves="2" seed="4" stitchTiles="stitch"');
    expect(grain).toContain('values="2.6 0 0 0 -0.82  2.6 0 0 0 -0.82  2.6 0 0 0 -0.8  0 0 0 0 1"');
    expect(streaks).toContain('baseFrequency="0.004 0.9" numOctaves="1" seed="11" stitchTiles="stitch"');
    expect(first.backgroundSize).toBe("1376px 344px, 688px 344px");
  });

  it("rasterises at the device pixel ratio, clamped to 1–3, without changing the tile's CSS size", async () => {
    for (const [ratio, side] of [
      [2, 'width="1376" height="688"'],
      [4, 'width="2064" height="1032"'],
      [0.4, 'width="688" height="344"'],
    ] as const) {
      pixelRatio(ratio);
      const tile = await loadTile();
      for (const svg of svgs(tile)) expect(svg).toContain(side);
      // The grain keeps its size on screen; only its sharpness follows the display.
      expect(tile.backgroundSize).toBe("1376px 344px, 688px 344px");
    }
  });

  it("hands CSS a cached bitmap when the canvas can encode one", async () => {
    const drawImage = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((done) => done(new Blob()));
    let next = 0;
    vi.spyOn(URL, "createObjectURL").mockImplementation(() => `blob:snow-${++next}`);

    const tile = await loadTile();
    expect(drawImage).toHaveBeenCalledTimes(2);
    expect(tile.backgroundImage).toMatch(/^url\("blob:snow-\d"\), url\("blob:snow-\d"\)$/);
  });

  it("never starts the work when the snow is not wanted", async () => {
    const useSnowTile = await freshHook();
    const { result } = renderHook(() => useSnowTile(false, true));
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(result.current).toBeUndefined();
    expect(decode).not.toHaveBeenCalled();
  });

  it("waits for a painted frame before drawing the idle snow", async () => {
    let frame: FrameRequestCallback | undefined;
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frame = callback;
      return 1;
    });
    const useSnowTile = await freshHook();
    const { result } = renderHook(() => useSnowTile(true, true));
    await new Promise((resolve) => setTimeout(resolve, 20));
    // The content in front of it gets the first frame to itself.
    expect(decode).not.toHaveBeenCalled();

    frame?.(0);
    await waitFor(() => expect(result.current).toBeDefined());
  });

  it("draws once per page, and a later surface has the tile on its first render", async () => {
    const useSnowTile = await freshHook();
    const first = renderHook(() => useSnowTile(true, false));
    await waitFor(() => expect(first.result.current).toBeDefined());
    const calls = decode.mock.calls.length;

    const later = renderHook(() => useSnowTile(true, true));
    expect(later.result.current).toBe(first.result.current);
    expect(decode.mock.calls.length).toBe(calls);
  });

  it("does not cache a failed tile: the next surface tries again", async () => {
    decode.mockImplementationOnce(() => Promise.reject(new Error("decode failed")));
    const useSnowTile = await freshHook();
    const first = renderHook(() => useSnowTile(true, false));
    await waitFor(() => expect(decode).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 20));
    // Decoration fails quietly: no snow, no throw.
    expect(first.result.current).toBeUndefined();
    first.unmount();

    const { result } = renderHook(() => useSnowTile(true, false));
    await waitFor(() => expect(result.current).toBeDefined());
  });
});
