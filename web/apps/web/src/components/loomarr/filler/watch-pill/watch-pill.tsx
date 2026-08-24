import { cn } from "@/lib/utils";
import type { WatchHealth, WatchPillProps } from "./watch-pill.type";

// WatchPill — the mock's `watchLine`, top-right of the Filler page header (§10 V38c).
//
// The card is the mock's, measured off the rendered prototype: `bg-card` on a 1px border, 8px
// radius, 8/13px padding, a 9px gap, 7px dot, 11px mono text.
//
// ⚠ **The DOT is a deliberate addition, not something the mock specifies.** The prototype draws it
// unconditionally green because a static mock has no failure states to show. Shipping that
// verbatim would mean a healthy green pulse on an install with every source switched off, or with
// nothing configured at all — the one moment the operator most needs to be told otherwise. The
// text says WHAT ("4 of 5 sources on"); the dot answers the question the text cannot: is this
// working right now? (Maintainer decision, 2026-08-02.)
const DOT: Record<WatchHealth, string> = {
  healthy: "bg-lock",
  attention: "bg-caution",
  unconfigured: "bg-static-500",
};

// ⚠ Only `healthy` pulses. Motion here means "running", so animating the other two would say the
// opposite of what they mean — and a page with a permanently throbbing amber dot trains an
// operator to stop seeing it.
//
// ⚠ `motion-safe:` gates the animation, matching OnAirIndicator. Reduced-motion users get the
// colour and the sr-only sentence, which carry the whole meaning without the movement.
const SR: Record<WatchHealth, string> = {
  healthy: "Watching your sources",
  attention: "Filler needs attention",
  unconfigured: "No sources set up yet",
};

const WatchPill = ({ status, health, className }: WatchPillProps) => (
  <div
    className={cn(
      "flex shrink-0 items-center gap-2 whitespace-nowrap rounded-lg border border-border bg-card px-3 py-2",
      className,
    )}
  >
    <span
      className={cn(
        "size-1.75 shrink-0 rounded-full",
        DOT[health],
        health === "healthy" && "motion-safe:animate-pulse",
      )}
      aria-hidden
    />
    <span className="font-mono text-muted-foreground text-xs">{status}</span>
    {/* ⚠ The dot is `aria-hidden`, so its meaning has to be said in words or it is invisible to a
        screen reader — a colour with no text equivalent is exactly the failure axe cannot catch
        (it sees a decorated span, not a missing sentence). */}
    <span className="sr-only">{SR[health]}</span>
  </div>
);

export { WatchPill };
