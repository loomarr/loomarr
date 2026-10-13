import { useEffect, useState } from "react";

// The analog snow, rasterised ONCE per page into small repeating bitmaps.
//
// It used to be an inline SVG filter over a 200% × 200% layer, so the browser ran two
// feTurbulence passes across four viewports' worth of device pixels — on a 3× phone, ~12 million
// pixels of noise — in the same frame as the sign-in form, and again whenever the layer re-painted.
// In the iPhone Simulator that frame landed about a second after the page's first paint. Now the
// same noise renders once into tiles (at the device pixel ratio, so the grain is the same size), the
// bitmaps are handed to CSS as repeating backgrounds, and the existing stepped transform moves them.
// Painting a cached bitmap is cheap; the noise is computed once per page.
//
// The snow is the analog snow the maintainer approved for the docs art ("E1", #1620): sharp,
// full-greyscale speckle — fractal noise stretched taller than wide (1.6 × 2.8), pushed through a
// hard gain (×2.6, −0.82) so it snaps between black and white — with faint horizontal streaks (a
// second, very wide noise) composited over it, the way a set's snow smears along the lines. Seeded,
// so a still frame is the same every render.
//
// Grain and streaks are two tiles, layered as two backgrounds (the streaks first, so they composite
// OVER the grain exactly as the filter's feComposite did). `stitchTiles` makes each noise wrap at
// its tile's edges, so the repeat has no seam, and it keeps the frequencies when a whole number of
// cycles fits the tile — hence the sizes:
// - grain: 688 × 344 px is 250 × 125 noise units, whole grain cycles to within a fraction of a %.
// - streaks: twice as wide, 500 units — exactly two cycles of their 0.004. One cycle would be
//   degenerate (both lattice ends wrap to the same gradient, and the streaks wash out). They are
//   smooth along x over ~700 px, so this tile renders at half horizontal resolution and is
//   stretched back: same pixels to paint as the grain tile, nothing visible lost.

// The docs figure draws E1 in a 200-unit viewBox shown ~550 px wide, so its grain is ~2.75× the
// filter's raw per-pixel frequency. Drawing the noise at that scale keeps the grain the maintainer
// approved, rather than a far finer one.
const GRAIN_SCALE = 2.75;

// CSS pixels. Whole numbers, so the grain bitmap maps 1:1 onto device pixels at every ratio.
const TILE_W = 688;
const TILE_H = 344;
const STREAK_W = TILE_W * 2;

const noiseSvg = (width: number, height: number, cssWidth: number, primitives: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${cssWidth / GRAIN_SCALE} ${TILE_H / GRAIN_SCALE}" preserveAspectRatio="none" shape-rendering="crispEdges">` +
  `<filter id="s" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB">${primitives}</filter>` +
  `<rect width="100%" height="100%" filter="url(#s)"/></svg>`;

const GRAIN =
  `<feTurbulence type="fractalNoise" baseFrequency="1.6 2.8" numOctaves="2" seed="4" stitchTiles="stitch"/>` +
  `<feColorMatrix type="matrix" values="2.6 0 0 0 -0.82  2.6 0 0 0 -0.82  2.6 0 0 0 -0.8  0 0 0 0 1"/>`;

const STREAKS =
  `<feTurbulence type="fractalNoise" baseFrequency="0.004 0.9" numOctaves="1" seed="11" stitchTiles="stitch"/>` +
  `<feColorMatrix type="matrix" values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  1.4 0 0 0 -0.62"/>`;

// One SVG drawn once into a bitmap. Without a bitmap (a browser that will not encode this canvas),
// the SVG itself still draws the same noise as a background; it only costs the filter per paint.
const rasterise = async (svg: string, width: number, height: number): Promise<string> => {
  const source = `data:image/svg+xml,${encodeURIComponent(svg)}`;
  const image = new Image(width, height);
  image.src = source;
  await image.decode();
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) return source;
  context.drawImage(image, 0, 0);
  const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve));
  return png ? URL.createObjectURL(png) : source;
};

interface SnowTile {
  backgroundImage: string;
  backgroundSize: string;
}

const renderTile = async (): Promise<SnowTile> => {
  const ratio = Math.min(Math.max(Math.round(window.devicePixelRatio || 1), 1), 3);
  const width = TILE_W * ratio;
  const height = TILE_H * ratio;
  const [streaks, grain] = await Promise.all([
    rasterise(noiseSvg(width, height, STREAK_W, STREAKS), width, height),
    rasterise(noiseSvg(width, height, TILE_W, GRAIN), width, height),
  ]);
  return {
    backgroundImage: `url("${streaks}"), url("${grain}")`,
    backgroundSize: `${STREAK_W}px ${TILE_H}px, ${TILE_W}px ${TILE_H}px`,
  };
};

// One tile per page, shared by every TvStatic: the login page's idle snow and the tuner's wash
// reuse the same bitmaps, and a remount shows them at once.
let tile: Promise<SnowTile> | undefined;
let ready: SnowTile | undefined;

const loadSnowTile = (): Promise<SnowTile> => {
  tile ??= renderTile().then((done) => {
    ready = done;
    return done;
  });
  return tile;
};

// The snow's background once it exists. `wanted: false` never starts the work (the idle snow under
// reduced motion, which is not shown). The first request waits until the page has painted a
// frame, so the content it sits behind — the sign-in form — is never held up by the snow.
const useSnowTile = (wanted: boolean): SnowTile | undefined => {
  const [snow, setSnow] = useState(ready);
  useEffect(() => {
    if (!wanted || snow) return;
    let live = true;
    const frame = requestAnimationFrame(() => {
      setTimeout(() => {
        loadSnowTile().then(
          (done) => live && setSnow(done),
          () => undefined, // No snow is the failure mode: it is decoration behind the content.
        );
      });
    });
    return () => {
      live = false;
      cancelAnimationFrame(frame);
    };
  }, [wanted, snow]);
  return snow;
};

export type { SnowTile };
export { useSnowTile };
