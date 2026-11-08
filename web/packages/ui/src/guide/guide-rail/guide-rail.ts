// The channel rail of a pointer guide takes a share of the window, between 200 and 280 points, so a
// long channel name ("Saturday Morning Cartoons") has room on a wide screen instead of a fixed width
// that cut it to "Saturday Morni…" (#2745).
const RAIL_SHARE = 0.18;
const RAIL_MIN = 200;
const RAIL_MAX = 280;

const guideRailWidth = (windowWidth: number): number =>
  Math.min(RAIL_MAX, Math.max(RAIL_MIN, Math.round(windowWidth * RAIL_SHARE)));

export { guideRailWidth };
