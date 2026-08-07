// A block on the strip. Deliberately NOT `SplitSegment` — the strip draws spans, and taking the
// whole domain type would couple a picture to every field a segment happens to carry (tags,
// transcript, dup-of). The editor passes what it has; the strip needs four things.
interface FilmstripSegment {
  // Stable across re-renders. The editor's draft key, so a merge/drop reconciles rather than
  // re-mounting every block.
  key: string;
  startMs: number;
  endMs: number;
  // Shown in the block's tooltip and its accessible name — the only way to tell blocks apart
  // without clicking one.
  name?: string;
  // Renders muted, matching the editor's own treatment: a segment the detector could not split
  // is evidence of a boundary it missed, not a clip.
  unsplittable?: boolean;
}

interface SegmentFilmstripProps {
  segments: FilmstripSegment[];
  // The block currently focused in the editor below, highlighted so the strip and the rows
  // agree about what is selected. Absent = nothing focused.
  activeKey?: string;
  // Clicking a block asks the editor to reveal that segment (the mock's `sp.focus`).
  onFocus?: (key: string) => void;
  className?: string;
}

export type { FilmstripSegment, SegmentFilmstripProps };
