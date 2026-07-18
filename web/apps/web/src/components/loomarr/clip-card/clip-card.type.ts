import type { ClipDTO } from "@loomarr/api";

// The clip data is the orval-generated ClipDTO (§12) — no hand-written mirror.
// ClipCardProps is web-specific (handlers, className).
interface ClipCardProps {
  clip: ClipDTO;
  onConfirmTags?: () => void;
  onTag?: () => void;
  className?: string;
}

export type { ClipCardProps };
