import type { PlaybarSegment, PlaybarWindow } from "../player-chrome.type";

interface PlaybarBox {
  id: string;
  segment: PlaybarSegment;
  x: number;
  width: number;
}

interface PlaybarLayout {
  boxes: readonly PlaybarBox[];
  /** The x of an instant, on the same geometry as the boxes (so the playhead sits on its block). */
  xAt: (ms: number) => number;
}

interface PlaybarLayoutOptions {
  /** The gap between two blocks. */
  gap: number;
  /** A break is never narrower than this, so it stays hoverable; programmes give up the difference. */
  minBreak: number;
}

/**
 * Lays the playbar out in pixels (§9.3): every block as wide as its share of the window, except
 * that a break never drops below a hoverable width, and the programmes give up the difference in
 * proportion to their own widths. Instants map onto the result piecewise, block by block, so the
 * playhead and the live line land on the block they belong to rather than drifting off it.
 */
const layoutPlaybar = (
  segments: readonly PlaybarSegment[],
  window: PlaybarWindow,
  width: number,
  { gap, minBreak }: PlaybarLayoutOptions,
): PlaybarLayout => {
  const span = window.endMs - window.startMs;
  const firstStart = segments[0]?.startMs ?? window.startMs;
  const lead = Math.max(0, firstStart - window.startMs);
  const usable = Math.max(0, width - gap * Math.max(0, segments.length - 1));
  const share = (ms: number) => (span > 0 ? (ms / span) * usable : 0);

  const sized = segments.map((segment) => {
    const natural = share(segment.stopMs - segment.startMs);
    return { natural, segment, width: segment.kind === "break" ? Math.max(natural, minBreak) : natural };
  });
  const deficit = sized.reduce((sum, b) => sum + b.width - b.natural, 0);
  const programmeTotal = sized.reduce((sum, b) => (b.segment.kind === "programme" ? sum + b.width : sum), 0);
  if (deficit > 0 && programmeTotal > 0) {
    const keep = Math.max(0, (programmeTotal - deficit) / programmeTotal);
    for (const b of sized) if (b.segment.kind === "programme") b.width *= keep;
  }

  let x = share(lead);
  const boxes: PlaybarBox[] = sized.map((b) => {
    const box = { id: b.segment.id, segment: b.segment, width: b.width, x };
    x += b.width + gap;
    return box;
  });

  const xAt = (ms: number) => {
    const first = boxes[0];
    if (!first) return share(Math.min(Math.max(ms - window.startMs, 0), span));
    if (ms <= first.segment.startMs)
      return lead > 0 ? (Math.max(0, ms - window.startMs) / lead) * first.x : first.x;
    const box = boxes.find((b) => ms < b.segment.stopMs);
    if (box) {
      const { startMs, stopMs } = box.segment;
      return box.x + ((ms - startMs) / (stopMs - startMs)) * box.width;
    }
    const last = boxes[boxes.length - 1] ?? first;
    return last.x + last.width;
  };

  return { boxes, xAt };
};

export { layoutPlaybar };
