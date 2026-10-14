interface RestartOverlayProps {
  /** True from the moment the restart is accepted until the app answers again. */
  restarting: boolean;
  /** True briefly after it returns, so success is confirmed rather than implied. */
  justCameBack?: boolean;
  /** Set when the app never came back — the operator has to act, so this does not fade. */
  failed?: string | null;
  className?: string;
}

export type { RestartOverlayProps };
