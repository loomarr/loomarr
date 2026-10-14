import type { HTMLAttributes } from "react";

type BadgeVariant = "caution" | "lock" | "neutral" | "onair" | "signal" | "suggest" | "tune";

type BadgeProps = HTMLAttributes<HTMLSpanElement> & {
  variant?: BadgeVariant;
  // Overrides the accent variants' default 15% composited tint. Callers that render a badge over
  // a non-standard background (e.g. invitation-join's card-on-card admin badge) can lower this to
  // keep the AA-safe text stop passing contrast at that specific call site.
  tintOpacity?: number;
};

export type { BadgeProps, BadgeVariant };
