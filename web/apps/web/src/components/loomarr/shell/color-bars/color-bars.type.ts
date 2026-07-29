type ColorBarsSize = "sm" | "lg";

interface ColorBarsProps {
  /** `lg` for the login/wizard hero (~200×14); `sm` for the sidebar lockup. */
  size?: ColorBarsSize;
  /**
   * Breathe each segment out of phase, so the card shimmers like a live signal instead of
   * sitting dead (§1). Opt-in, and only for IDLE surfaces — the guide's "Dead air" empty
   * state. The brand lockup in the sidebar and the login/wizard heroes stay still: a mark
   * that pulses everywhere is noise, and next to real data it reads as a status.
   */
  breathe?: boolean;
  className?: string;
}

export type { ColorBarsProps, ColorBarsSize };
