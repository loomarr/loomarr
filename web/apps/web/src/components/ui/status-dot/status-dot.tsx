import { semanticColors } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { StatusDotProps, StatusTone } from "./status-dot.type";

// StatusDot — the small round state indicator (§5.1c), backed by @loomarr/design-system's shared
// token source (#970 PR B). The dot's own StatusDot primitive renders through Tamagui, which has
// no polymorphic element and only forwards arbitrary HTML attributes (data-testid, title) as a
// best-effort className passthrough — not the exact-override guarantee this file's callers rely
// on (`className="size-1.5"` overriding the default size via tailwind-merge). This component keeps
// the legacy <span> and its full prop contract, and sources only the colour values from the
// design-system's semantic tokens, which are confirmed byte-identical to this app's own legacy
// CSS variables (e.g. `semanticColors.guide.onAir` === `--color-onair`).
//
// ⚠ OnAirIndicator deliberately does NOT use this. It renders a larger dot with an expanding
// `ping` ring — a richer treatment for the app's most prominent live signal — and flattening
// that into the generic dot would be homogenisation rather than de-duplication. A primitive
// earns its callers; it does not conscript them.
//
// The pulse is reserved for `live` alone: it means "this is happening right now", so spending
// it on a pending or warning state would cost the dot its one piece of motion vocabulary. Under
// prefers-reduced-motion the animation stops (§2.4) — which is why colour is never the only
// signal, and the label is required.
// ⚠ `error` and `live` share a colour and differ ONLY in the pulse. That is the point:
// onair red means "right now" when it moves and "this failed" when it does not. A failed
// background job must not borrow `live` to get the red — a job that errored hours ago has
// nothing happening, and pulsing says otherwise. Added for the Tasks page, which had built
// a static red dot by hand for exactly this state.
//
// ⚠ `pending` is the LIGHTER amber (signal-400, not signal). Callers wanting brand amber
// for "in progress" want `live` — motion is what distinguishes work happening now from
// work merely queued, and that is the distinction the two tones encode.
const TONE_COLOR: Record<StatusTone, string> = {
  live: semanticColors.guide.onAir,
  pending: semanticColors.guide.reconciling,
  ok: semanticColors.state.success,
  warn: semanticColors.state.warning,
  error: semanticColors.guide.onAir,
  off: semanticColors.action.disabled,
  // The redesign's Home and Requests rows (#1659 web mock). Each names what the row is WAITING
  // on, which is what the colour tells an operator at a glance:
  // `attention` — waiting on a person (an approval, a request to review); the suggest colour.
  attention: semanticColors.accent.suggest,
  // `progress` — on its way, nobody needs to act (a download, a lineup being found).
  progress: semanticColors.state.info,
  // `notice` — something to do when convenient (a restart to finish saving settings). Brand
  // amber, static: not `pending`'s lighter queued amber, and not `live`'s motion.
  notice: semanticColors.action.primary,
};

const StatusDot = ({ tone, label, className, style, ...rest }: StatusDotProps) => (
  <span
    // role="img" so the label is announced: aria-label is not supported on a bare <span>,
    // whose implicit role is generic. An unlabelled dot is decorative and gets aria-hidden.
    {...(label ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
    className={cn("size-2", tone === "live" && "motion-safe:animate-pulse", className)}
    style={{
      backgroundColor: TONE_COLOR[tone],
      borderRadius: 9999,
      display: "inline-block",
      flexShrink: 0,
      ...style,
    }}
    {...rest}
  />
);

export { StatusDot };
