interface VolumeControlProps {
  /** Current volume, 0–1. Controlled by the parent (which mirrors it onto the media element). */
  volume: number;
  /** Whether audio is muted. Muting is independent of the level, so it can be restored on unmute. */
  muted: boolean;
  /** Set the volume level (0–1). */
  onVolumeChange: (volume: number) => void;
  /** Set the muted state. */
  onMutedChange: (muted: boolean) => void;
}

export type { VolumeControlProps };
