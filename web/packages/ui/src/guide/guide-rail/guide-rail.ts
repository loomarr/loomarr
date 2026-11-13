// The channel rail of a pointer guide takes a share of the window, between 200 and 300 points, so a
// long channel name ("Saturday Morning Cartoons") has room on a wide screen instead of a fixed width
// that cut it to "Saturday Morni…" (#2745).
//
// A fifth of the window, not 18%: the rail also holds the decade bar, the icon, the number, the
// on-air dot and the menu, which left the name about 87 points at 1280 and cut "Still downloading"
// under it (#3410). At a fifth the name has about 113 points at 1280 and 145 at 1440.
const RAIL_SHARE = 0.2;
const RAIL_MIN = 200;
const RAIL_MAX = 300;

const guideRailWidth = (windowWidth: number): number =>
  Math.min(RAIL_MAX, Math.max(RAIL_MIN, Math.round(windowWidth * RAIL_SHARE)));

export { guideRailWidth };
