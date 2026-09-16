import type { ClipDTO } from "@loomarr/api/models/clipDTO";

// The clip data is the orval-generated ClipDTO (§12) — no hand-written mirror.
// ClipCardProps is web-specific (handlers, className).
interface ClipCardProps {
  clip: ClipDTO;
  // Start compilation-split detection (§10 V34). Admin-only at the call site; absent
  // renders no split action.
  onSplit?: () => void;
  // Detection for THIS clip is in flight — disables the split action so a slow decode
  // can't be queued twice.
  splitPending?: boolean;
  // Bulk selection (V35). ⚠ `onToggleSelect` is what makes the card selectable at ALL — absent,
  // no checkbox renders, which is how a member (who cannot bulk-edit) sees the same card
  // without a control that would 403. `selected` alone does nothing.
  selected?: boolean;
  onToggleSelect?: () => void;
  // Read-only inspection from title or thumbnail; callers own the shared panel.
  onOpen?: () => void;
  className?: string;
}

export type { ClipCardProps };
