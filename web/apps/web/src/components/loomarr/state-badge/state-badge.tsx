import { cva } from "class-variance-authority";
import { cn } from "@/lib";
import type { ProvisioningState, StateBadgeProps } from "./state-badge.type";

// StateBadge — the provisioning-lifecycle chip (§3). Mono, uppercase, tinted per
// the §2.1 lifecycle mapping. The visible text is letter-spaced shout; the
// sentence-case aria-label is what a screen reader speaks (§5.3). None of these
// states use onair/suggest, so no -300 stop is needed — tune/lock/caution clear
// AA on their 15% tints.
const badge = cva(
  "inline-flex items-center rounded-sm px-1.5 py-0.5 font-medium font-mono text-[11px] uppercase tracking-wide",
  {
    variants: {
      state: {
        wanted: "bg-static-800 text-static-400",
        requested: "bg-static-800 text-static-400",
        downloading: "bg-tune-tint-15 text-tune",
        available: "bg-lock-tint-15 text-lock",
        unavailable: "bg-static-800 text-static-400 line-through",
        drift: "bg-caution-tint-15 text-caution",
      },
    },
    defaultVariants: { state: "wanted" },
  },
);

const LABEL: Record<ProvisioningState, string> = {
  wanted: "Wanted",
  requested: "Requested",
  downloading: "Downloading",
  available: "Available",
  unavailable: "Unavailable",
  drift: "Drift",
};

// The uppercase + letter-spacing is CSS-only, so the DOM text stays sentence-case
// ("Downloading") — that IS the accessible name a screen reader speaks (§5.3), so
// no aria-label is needed (and aria-label on a generic <span> isn't ARIA-valid).
const StateBadge = ({ state, className }: StateBadgeProps) => (
  <span className={cn(badge({ state }), className)}>{LABEL[state]}</span>
);

export { StateBadge };
