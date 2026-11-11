// PROTOTYPE — throwaway (Direction mocks, 2026-11-09)
// The channel watermark ("bug") that playout burns into the picture, and the arithmetic a player
// needs to keep its own chrome out of that corner. The same rule variant H drew (chrome-tune.tsx has
// its own copy; this one is for variants built on G, which H's file imports from).
//
// Playout's defaults (internal/playout/watermark): 6% of the picture tall, inset 5% from the
// picture's edges, white at 40%, top right. It is part of the video, so the player can't move it.

/** The channel's watermark corner, as the channel API reports it; "off" when the bug is disabled. */
type BugCorner = "top-right" | "top-left" | "bottom-right" | "bottom-left" | "off";

// The device frames (shared.tsx's ProtoFrame).
const FRAME: Record<string, { H: number; W: number }> = {
  desktop: { H: 900, W: 1440 },
  landscape: { H: 390, W: 844 },
  portrait: { H: 844, W: 390 },
  tv: { H: 540, W: 960 },
};

/** The 16:9 picture inside the frame, letterboxed (or pillarboxed) the way real video is. */
const pictureOf = (device: string) => {
  const f = FRAME[device] ?? { H: 900, W: 1440 };
  const w = Math.min(f.W, (f.H * 16) / 9);
  const h = (w * 9) / 16;
  return { h, w, x: (f.W - w) / 2, y: (f.H - h) / 2 };
};

const BUG_SIZE = 0.06;
const BUG_MARGIN = 0.05;
const BUG_GAP = 16;

/**
 * Where chrome on the bug's edge (top or bottom) must start, in px from that edge, to clear the bug,
 * or `base` when it already clears. `reach` is how far the chrome extends from that edge. On a
 * portrait phone the picture is a strip mid-screen, so the bug is nowhere near and nothing moves.
 */
const clearOfBug = (device: string, base: number, reach: number): number => {
  const p = pictureOf(device);
  const near = p.y + BUG_MARGIN * p.h;
  const far = near + BUG_SIZE * p.h;
  if (near >= base + reach + BUG_GAP || far <= base - BUG_GAP) return base;
  return Math.max(base, Math.ceil((far + BUG_GAP) / 8) * 8);
};

/** A stand-in for the burned-in bug, inside the picture's rectangle (not the screen's). */
const ChannelBug = ({
  corner,
  device,
  name,
}: {
  corner: Exclude<BugCorner, "off">;
  device: string;
  name: string;
}) => {
  const p = pictureOf(device);
  return (
    <div
      aria-hidden
      style={{ height: p.h, left: p.x, pointerEvents: "none", position: "absolute", top: p.y, width: p.w }}
    >
      <span
        style={{
          color: "rgba(255,255,255,0.4)",
          fontSize: BUG_SIZE * p.h * 0.75,
          fontWeight: 800,
          letterSpacing: "0.02em",
          lineHeight: `${BUG_SIZE * p.h}px`,
          position: "absolute",
          textTransform: "uppercase",
          whiteSpace: "nowrap",
          [corner.startsWith("top") ? "top" : "bottom"]: BUG_MARGIN * p.h,
          [corner.endsWith("right") ? "right" : "left"]: BUG_MARGIN * p.w,
        }}
      >
        {name}
      </span>
    </div>
  );
};

export type { BugCorner };
export { ChannelBug, clearOfBug };
