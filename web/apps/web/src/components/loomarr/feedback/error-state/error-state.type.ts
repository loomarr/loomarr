interface ErrorStateProps {
  error: unknown;
  onRetry?: () => void;
  /** The retry is in flight: the button stays (so focus stays on it), reads "Trying again…" and waits. */
  retrying?: boolean;
  /** Pads the block by 16px instead of 32, for a dialog body that already sets its own inset. */
  compact?: boolean;
}

export type { ErrorStateProps };
