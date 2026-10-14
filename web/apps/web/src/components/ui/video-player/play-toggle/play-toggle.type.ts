interface PlayToggleProps {
  /** Whether the media is currently playing — drives the icon and the accessible name. */
  playing: boolean;
  /** Toggle playback. */
  onToggle: () => void;
}

export type { PlayToggleProps };
