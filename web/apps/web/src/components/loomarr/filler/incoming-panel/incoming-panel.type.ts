import type { IncomingAskDTO, IncomingReelDTO } from "@loomarr/api";

interface IncomingPanelProps {
  // The server's answer, verbatim (contract 1:1) — the two halves of one read.
  asks: IncomingAskDTO[];
  reels: IncomingReelDTO[];
  // Confirms an era the tagger guessed. ⚠ Confirming is what CLEARS the suggestion (§10 V34),
  // and it goes through the ordinary tag edit so the grounding rule has one implementation.
  onConfirmEra?: (clip: IncomingAskDTO) => void;
  // Opens the full tag editor for a clip whose guess was wrong, or which has no guess at all.
  onEditTags?: (clip: IncomingAskDTO) => void;
  // Removes a clip from the catalog. ⚠ A tombstone, never a file delete.
  onDismiss?: (clip: IncomingAskDTO) => void;
  // Which clip path is currently being written, so its row can say so.
  busyPath?: string;
  className?: string;
}

export type { IncomingPanelProps };
