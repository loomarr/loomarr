import type { Clip, ClipAudience, ClipKind } from "@loomarr/core";

// The Clip data contract lives in @loomarr/core — shared with packages/fixtures and
// the future mobile app (§4.2). ClipCardProps is web-specific (handlers, className)
// and stays here; the data types are re-exported so siblings keep their barrel imports.
interface ClipCardProps {
  clip: Clip;
  onConfirmTags?: () => void;
  onTag?: () => void;
  className?: string;
}

export type { Clip, ClipAudience, ClipCardProps, ClipKind };
