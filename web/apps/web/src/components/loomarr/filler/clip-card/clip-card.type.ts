import type { ClipDTO } from "@loomarr/api";

// The clip data is the orval-generated ClipDTO (§12) — no hand-written mirror.
// ClipCardProps is web-specific (handlers, className).
interface ClipCardProps {
  clip: ClipDTO;
  onConfirmTags?: () => void;
  onTag?: () => void;
  // Pin this clip into a channel's filler (P3 cohesion) — the catalog → channel bridge.
  // Admin-only at the call site; absent renders no pin action.
  onPin?: () => void;
  // One-click confirm of an UNGROUNDED AI era guess (§10 V34): the year is in none of the
  // clip's text signals, so it sits on `suggestedEra` until a human says yes. Admin-only
  // at the call site; without it the suggestion renders as a badge with no action.
  onConfirmEra?: () => void;
  // Start compilation-split detection (§10 V34). Admin-only at the call site; absent
  // renders no split action.
  onSplit?: () => void;
  // Detection for THIS clip is in flight — disables the split action so a slow decode
  // can't be queued twice.
  splitPending?: boolean;
  className?: string;
}

export type { ClipCardProps };
