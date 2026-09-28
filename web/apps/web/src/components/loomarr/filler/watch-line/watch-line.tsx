import { cn } from "@/lib/utils";
import type { WatchHealth, WatchLineProps } from "./watch-line.type";

// WatchLine — the web mock's `watchLine` (#1659): a plain mono line under the Filler page's
// description, with a 7px status dot. It was a bordered pill at the header's top right (§10
// V38c); the web mock draws it unboxed under the description, so it is a line now.
//
// ⚠ **The dot's colour follows the server's health verdict.** The web mock does the same (green and
// pulsing when healthy, caution and still when not). Before it did, a static mock drew the dot
// unconditionally green, and shipping that would have shown a healthy pulse on an install with every
// source switched off. The text says WHAT ("3 of 4 sources on"); the dot answers whether it is
// working right now. (Maintainer decision, 2026-08-02.)
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
  attention: "Sources need attention",
  unconfigured: "No sources set up yet",
};

const WatchLine = ({ status, health, className }: WatchLineProps) => (
  <p className={cn("mt-2 flex min-w-0 items-center gap-2", className)}>
    <span
      className={cn(
        "size-1.75 shrink-0 rounded-full",
        DOT[health],
        health === "healthy" && "motion-safe:animate-pulse",
      )}
      aria-hidden
    />
    <span className="min-w-0 font-mono text-muted-foreground text-xs">{status}</span>
    {/* ⚠ The dot is `aria-hidden`, so its meaning has to be said in words or it is invisible to a
        screen reader — a colour with no text equivalent is exactly the failure axe cannot catch
        (it sees a decorated span, not a missing sentence). */}
    <span className="sr-only">{SR[health]}</span>
  </p>
);

export { WatchLine };
