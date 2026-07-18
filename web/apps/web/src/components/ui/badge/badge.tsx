import { cva } from "class-variance-authority";
import { cn } from "@/lib";
import type { BadgeProps } from "./badge.type";

// shadcn/ui Badge (new-york), restyled via Test Card tokens only (§3, Layer 1).
// Badges carry machine data, so mono + uppercase (§2.2). Accent variants pair the
// 15% composited tint with the AA-safe text stop — onair/suggest use their -300
// stops (§2.1 badge rule), which the token generator's contrast gate verifies in CI.
const badgeVariants = cva(
  "inline-flex w-fit items-center rounded-sm px-1.5 py-0.5 font-medium font-mono text-[11px] uppercase tracking-wide",
  {
    variants: {
      variant: {
        neutral: "bg-static-800 text-static-400",
        tune: "bg-tune-tint-15 text-tune",
        lock: "bg-lock-tint-15 text-lock",
        caution: "bg-caution-tint-15 text-caution",
        onair: "bg-onair-tint-15 text-onair-300",
        suggest: "bg-suggest-tint-15 text-suggest-300",
        signal: "bg-signal-tint-15 text-signal",
      },
    },
    defaultVariants: { variant: "neutral" },
  },
);

const Badge = ({ className, variant, ...props }: BadgeProps) => (
  <span className={cn(badgeVariants({ variant }), className)} {...props} />
);

export { Badge, badgeVariants };
