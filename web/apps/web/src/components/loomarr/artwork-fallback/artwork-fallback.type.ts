interface ArtworkFallbackProps {
  // The channel this artwork would have belonged to, for the monogram. Absent when the slot
  // (a title poster with no channel yet) has nothing to derive one from.
  channel?: { name: string; number: number };
  className?: string;
}

export type { ArtworkFallbackProps };
