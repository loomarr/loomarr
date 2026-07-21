type BrandLockupVariant = "hero" | "compact";

interface BrandLockupProps {
  /** `hero` = centered, large wordmark + tagline (login/wizard); `compact` = horizontal, small (sidebar). */
  variant?: BrandLockupVariant;
  /** Show the "always something on" tagline (hero only). Default true for hero, ignored for compact. */
  tagline?: boolean;
  className?: string;
}

export type { BrandLockupProps, BrandLockupVariant };
