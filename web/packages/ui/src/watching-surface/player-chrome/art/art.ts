import type { Density } from "@loomarr/design-system";

// Title art in the player (§9.2, #2773, #2774): a title's clear logo in the band and its backdrop
// behind the station ident. The player asks the image service for the rung it will actually draw,
// never a fixed size: a phone gets a small rung and a 2x laptop gets the sharp one.

/** The served image's fields the player needs (a subset of the API's `ImageDTO`). */
interface PlayerArt {
  /** The original's height in pixels. */
  height: number;
  /** A same-origin path for the fallback `<img>`. */
  src: string;
  /** Same-origin WebP `srcset` of the role's rungs, `path 300w, path 780w`; empty when there is none. */
  srcSetWebp: string;
  /** The original's width in pixels. */
  width: number;
}

/**
 * The URL of the smallest rung that is at least `cssWidth` × `pixelRatio` device pixels wide, or the
 * widest the image has (the server never lists a rung larger than the source, so that is never an
 * upscale). An image with no `srcset` is its `src`.
 */
const rungFor = (
  art: Pick<PlayerArt, "src" | "srcSetWebp">,
  cssWidth: number,
  pixelRatio: number,
): string => {
  const rungs = art.srcSetWebp
    .split(",")
    .map((entry) => entry.trim().split(/\s+/))
    .flatMap(([url, descriptor]) => {
      const width = Number.parseInt(descriptor ?? "", 10);
      return url && Number.isFinite(width) ? [{ url, width }] : [];
    })
    .sort((a, b) => a.width - b.width);
  if (rungs.length === 0) return art.src;
  const wanted = Math.ceil(cssWidth * pixelRatio);
  return (rungs.find((rung) => rung.width >= wanted) ?? rungs[rungs.length - 1])?.url ?? art.src;
};

// Logos are sized by area, not height (§9.2): at one height a nearly square logo comes out tiny beside
// a wide wordmark. Each is fitted to the same visual area, inside a box no logo may leave.
const LOGO_AREA: Record<"pointer" | "touch", { area: number; maxHeight: number; maxWidth: number }> = {
  pointer: { area: 7200, maxHeight: 88, maxWidth: 280 },
  touch: { area: 2800, maxHeight: 56, maxWidth: 200 },
};

/** The size to draw a logo of this shape at, in px: the same area for any shape, within the box. */
const logoSize = (
  art: Pick<PlayerArt, "height" | "width">,
  density: Density,
): { height: number; width: number } => {
  const { area, maxHeight, maxWidth } = LOGO_AREA[density === "touch" ? "touch" : "pointer"];
  const ratio = art.width > 0 && art.height > 0 ? art.width / art.height : 1;
  const width = Math.sqrt(area * ratio);
  const height = Math.sqrt(area / ratio);
  const fit = Math.min(1, maxWidth / width, maxHeight / height);
  return { height: Math.round(height * fit), width: Math.round(width * fit) };
};

export type { PlayerArt };
export { logoSize, rungFor };
