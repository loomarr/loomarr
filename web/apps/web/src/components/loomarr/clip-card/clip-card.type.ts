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
  className?: string;
}

export type { ClipCardProps };
