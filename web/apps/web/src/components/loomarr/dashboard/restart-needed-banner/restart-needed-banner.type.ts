interface RestartNeededBannerProps {
  /**
   * The boot-time settings whose saved value differs from the one this process is
   * running. Empty ⇒ the banner does not render at all.
   */
  pendingKeys: string[];
  /** Scroll/focus the restart control. The banner never restarts directly — the
   *  consequences are stated in the confirm dialog, and a one-click restart from a banner
   *  would skip them. */
  onGoToRestart: () => void;
  className?: string;
}

export type { RestartNeededBannerProps };
