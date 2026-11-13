interface ErrorStateProps {
  error: unknown;
  onRetry?: () => void;
  /** The retry is in flight: the button stays (so focus stays on it), reads "Trying again…" and waits. */
  retrying?: boolean;
  className?: string;
}

export type { ErrorStateProps };
