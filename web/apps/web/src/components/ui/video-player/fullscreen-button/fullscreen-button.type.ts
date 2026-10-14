interface FullscreenButtonProps {
  /** Whether the player is currently fullscreen — drives the icon and the accessible name. */
  active: boolean;
  /** Enter/exit fullscreen. */
  onToggle: () => void;
}

export type { FullscreenButtonProps };
