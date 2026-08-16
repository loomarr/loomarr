interface SegmentPreviewProps {
  // The COMPOSITE's content hash — the parent reel, not the segment's.
  //
  // ⚠ A proposed segment has no bytes of its own until `confirm` cuts it, which is the whole
  // reason this plays a WINDOW of the parent (§10 V45 keeps the composite on disk precisely so it
  // is still there to play).
  clipHash: string;
  startMs: number;
  endMs: number;
  // 1-based row ordinal. Drives the element ids and the accessible name, so it must match the
  // "#N" marker the operator can see beside it.
  position: number;
  // id of the VISIBLE "#N" marker in the row. The tile has no text of its own, and an sr-only-only
  // name drifts from what is on screen — so the name is composed from a fixed verb plus this.
  labelledBy: string;
  // Controlled by the editor, which keeps at most ONE preview open: two expanded previews are two
  // audio streams, and 52 mounted <video>s are 52 open range requests against a 20-minute file.
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Safe here and only here: the toggle click IS the gesture browsers require. Stories omit it —
  // a moving playhead changes the snapshot on every visual run.
  autoPlay?: boolean;
  className?: string;
}

export type { SegmentPreviewProps };
