import type { HTMLAttributes } from "react";

type BadgeVariant = "caution" | "lock" | "neutral" | "onair" | "signal" | "suggest" | "tune";

type BadgeProps = HTMLAttributes<HTMLSpanElement> & { variant?: BadgeVariant };

export type { BadgeProps, BadgeVariant };
