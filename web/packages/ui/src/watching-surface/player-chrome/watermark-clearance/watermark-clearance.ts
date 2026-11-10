// Keeping the chrome out of the channel watermark (§9.6, #2772). Playout burns each channel's bug
// into the video (default top-right, 6 % of the picture tall, inset 5 %), so the player cannot move
// it. It reads the bug's corner, works out the 16:9 picture's rectangle inside the frame (letterboxed
// as the video really is) and the bug's box inside that, and starts its own chrome clear of the box.
// Nothing moves when the box is already clear: on a portrait phone the picture is a strip mid-screen.

type WatermarkCorner = "bottom-left" | "bottom-right" | "top-left" | "top-right";

/** The channel's resolved bug: where it is, how tall (a share of the picture) and how far in. */
interface PlayerWatermark {
  corner: WatermarkCorner;
  /** Inset from the picture's edges, a share of the picture's height (and width). */
  margin: number;
  /** The bug's height as a share of the picture's height. */
  size: number;
}

/** The channel's watermark policy as the API sends it: every field is optional, absent is the default. */
interface WatermarkPolicyLike {
  corner?: WatermarkCorner;
  enabled?: boolean;
  margin?: number;
  size?: number;
}

interface Frame {
  height: number;
  width: number;
}

// Playout's defaults (`internal/playout/watermark`, the API's field docs).
const DEFAULT_CORNER: WatermarkCorner = "top-right";
const DEFAULT_SIZE = 0.06;
const DEFAULT_MARGIN = 0.05;
// Air between the bug and the chrome that steps aside for it, and the grid it steps on.
const GAP = 16;
const GRID = 8;

/** The bug to keep clear of, or undefined when the channel's is switched off. */
const watermarkOf = (policy: WatermarkPolicyLike | undefined): PlayerWatermark | undefined =>
  policy?.enabled === false
    ? undefined
    : {
        corner: policy?.corner ?? DEFAULT_CORNER,
        margin: policy?.margin ?? DEFAULT_MARGIN,
        size: policy?.size ?? DEFAULT_SIZE,
      };

/** The 16:9 picture inside the frame, letterboxed or pillarboxed the way real video is. */
const pictureIn = (frame: Frame) => {
  const width = Math.min(frame.width, (frame.height * 16) / 9);
  const height = (width * 9) / 16;
  return { height, width, x: (frame.width - width) / 2, y: (frame.height - height) / 2 };
};

/**
 * Where chrome on the bug's edge must start, in px from that edge, to clear the bug: `base` when it
 * already does. `extent` is how far the chrome reaches from `base` (the top row's 48, or the band's
 * and keys' height). `edge` is the frame edge the chrome hangs from, and `side` the half it occupies:
 * only a bug on that edge AND side is in its way (#3397), so a top-left bug leaves the top-right row
 * alone. Chrome spanning the frame (the band and keys) is `both`.
 */
const clearOfWatermark = ({
  base,
  edge,
  extent,
  frame,
  side,
  watermark,
}: {
  base: number;
  edge: "bottom" | "top";
  extent: number;
  frame: Frame;
  side: "both" | "left" | "right";
  watermark: PlayerWatermark | undefined;
}): number => {
  if (!watermark || frame.width <= 0 || frame.height <= 0) return base;
  const onThisEdge = watermark.corner.startsWith(edge);
  const onThisSide = side === "both" || watermark.corner.endsWith(side);
  if (!onThisEdge || !onThisSide) return base;
  const picture = pictureIn(frame);
  const gapToFrame = edge === "top" ? picture.y : frame.height - (picture.y + picture.height);
  const near = gapToFrame + watermark.margin * picture.height;
  const far = near + watermark.size * picture.height;
  // Clear already: the bug sits beyond the chrome's reach (or the chrome stops short of it).
  if (near >= base + extent + GAP || far + GAP <= base) return base;
  return Math.max(base, Math.ceil((far + GAP) / GRID) * GRID);
};

export type { Frame, PlayerWatermark, WatermarkCorner, WatermarkPolicyLike };
export { clearOfWatermark, pictureIn, watermarkOf };
