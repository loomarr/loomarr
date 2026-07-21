type ColorBarsSize = "sm" | "lg";

interface ColorBarsProps {
  /** `lg` for the login/wizard hero (~200×14); `sm` for the sidebar lockup. */
  size?: ColorBarsSize;
  className?: string;
}

export type { ColorBarsProps, ColorBarsSize };
