import type { ClipDTO } from "@loomarr/api/models/clipDTO";

// The clip data is the orval-generated ClipDTO (§12) — no hand-written mirror.
interface ClipRowProps {
  clip: ClipDTO;
  // Bulk selection (V35). ⚠ As on ClipCard, `onToggleSelect` is what makes the row selectable
  // at ALL — absent, no checkbox renders, which is how a member (who cannot bulk-edit) sees
  // the same row without a control that would 403. `selected` alone does nothing.
  selected?: boolean;
  onToggleSelect?: () => void;
  className?: string;
}

export type { ClipRowProps };
