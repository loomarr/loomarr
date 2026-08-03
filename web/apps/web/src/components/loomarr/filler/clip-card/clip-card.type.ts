import type { ClipDTO } from "@loomarr/api";

// The clip data is the orval-generated ClipDTO (§12) — no hand-written mirror.
// ClipCardProps is web-specific (handlers, className).
interface ClipCardProps {
  clip: ClipDTO;
  onConfirmTags?: () => void;
  onTag?: () => void;
  // Pin this clip into a channel's filler (P3 cohesion) — the catalog → channel bridge.
  // Admin-only at the call site; absent renders no pin action.
  onPin?: () => void;
  // One-click confirm of an UNGROUNDED AI era guess (§10 V34): the year is in none of the
  // clip's text signals, so it sits on `suggestedEra` until a human says yes. Admin-only
  // at the call site; without it the suggestion renders as a badge with no action.
  onConfirmEra?: () => void;
  // Retag ONE field by clicking its chip (the v2 mock's cycleEra/cycleAud/cycleCat).
  // Admin-only at the call site; absent renders the chips as plain, non-interactive badges.
  //
  // ⚠ The caller is responsible for sending the clip's OTHER tags with the change: the BE's
  // UpdateClipTags overwrites era, audience and category together, so a patch carrying only
  // the cycled field wipes the rest. FillerPage's `retag` is the one place that assembles it.
  onCycle?: (change: Partial<Pick<ClipDTO, "era" | "audience" | "category">>) => void;
  // Start compilation-split detection (§10 V34). Admin-only at the call site; absent
  // renders no split action.
  onSplit?: () => void;
  // Detection for THIS clip is in flight — disables the split action so a slow decode
  // can't be queued twice.
  splitPending?: boolean;
  // Bulk selection (V35). ⚠ `onToggleSelect` is what makes the card selectable at ALL — absent,
  // no checkbox renders, which is how a member (who cannot bulk-edit) sees the same card
  // without a control that would 403. `selected` alone does nothing.
  selected?: boolean;
  onToggleSelect?: () => void;
  // Open this clip in the player (V39). Member-safe, unlike every other action here: watching a
  // clip changes nothing, and `media` is already member-readable — so this is offered to anyone
  // who can see the card at all.
  //
  // Absent renders no play control anywhere on the card, which is the honest degraded state for a
  // caller that has nowhere to open a player.
  onPlay?: () => void;
  className?: string;
}

export type { ClipCardProps };
