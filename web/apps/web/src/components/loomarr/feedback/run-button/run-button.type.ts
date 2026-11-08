interface RunButtonProps {
  // True while the action is in flight. Compute it with `useRunFeedback` unless the surface
  // genuinely has a reliable server-side signal — see that hook for why it usually does not.
  busy: boolean;
  onRun: () => void;
  label?: string; // default "Run now"
  busyLabel?: string; // default "Running…"
  // The caller's own reason the run is unavailable, separate from busy.
  disabled?: boolean;
}

export type { RunButtonProps };
