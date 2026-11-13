import { Badge, type BadgeTone } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { ProvisioningState, StateBadgeProps } from "./state-badge.type";

// StateBadge — the provisioning-lifecycle chip (§3), composed from the Badge primitive.
//
// ⚠ DO NOT FORK `Badge` (its own `cva`, base classes or tint mapping). Two components
// implementing "small tinted chip with mono uppercase text" would encode the §2.1 badge rule —
// the contrast calibration the doc calls "learned the hard way, twice" — in two places that do
// not know about each other. Fix a stop in one and the other stays wrong, and the contrast gate
// cannot tell: it validates token PAIRINGS, not which components consume them.
//
// A fork could look safe while none of these states maps to onair or suggest (the two accents
// that need a -300 stop). Accidentally safe is not safe — a future state mapping to onair would
// get it wrong, with nothing to catch it.
//
// What stays here is the part that IS domain knowledge: which lifecycle state reads as which
// tone, and the struck-through treatment for a title Loomarr gave up on.
const TONE: Record<ProvisioningState, BadgeTone> = {
  wanted: "neutral",
  requested: "neutral",
  downloading: "tune",
  available: "lock",
  unavailable: "neutral",
  drift: "caution",
};

const LABEL: Record<ProvisioningState, string> = {
  wanted: "Wanted",
  requested: "Requested",
  downloading: "Downloading",
  available: "Available",
  unavailable: "Unavailable",
  drift: "Drift",
};

// The DOM text is the sentence-case label ("Downloading") — that IS the accessible name a screen
// reader speaks (§5.3), so no aria-label is needed (and aria-label on a generic <span> isn't
// ARIA-valid).
const StateBadge = ({ state, className }: StateBadgeProps) => (
  <Badge
    shape="square"
    tone={TONE[state]}
    // Loomarr gave up on this title (§4). The strike-through is the one visual the base chip
    // has no notion of, and it is genuinely domain-specific.
    className={cn(state === "unavailable" && "line-through", className)}
  >
    {LABEL[state]}
  </Badge>
);

export { StateBadge };
