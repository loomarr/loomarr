interface ArtworkFallbackProps {
  // The channel this artwork would have belonged to: its name is the wordmark and its number picks
  // the hue. Absent when the slot (a title poster with no channel yet) has none.
  channel?: { name: string; number: number };
  // The words to set when there is no channel, such as a title's own name.
  label?: string;
  // Picks the hue when there is no channel, so the same title always gets the same colour.
  seed?: number;
  className?: string;
}

export type { ArtworkFallbackProps };
