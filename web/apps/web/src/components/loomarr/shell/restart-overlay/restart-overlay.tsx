import { Check, Loader2 } from "lucide-react";
import { cn } from "@/lib";
import type { RestartOverlayProps } from "./restart-overlay.type";

// RestartOverlay — the app is unusable while it restarts, so say so and mean it (§9.2, V13).
//
// While the server rebuilds, every request fails. Left interactive, the page invites
// clicks that produce error toasts for actions that were never going to work — which
// reads as "I broke it" rather than "it is coming back". The overlay dims the app,
// swallows input, and states what is happening.
//
// ⚠ **It covers the whole shell, not one page.** The control is on the Dashboard, but an
// operator can navigate away the instant they click it, and a per-page overlay would
// vanish with the route and leave a dead app looking merely broken.
//
// ⚠ **Success is stated, not implied by disappearance.** The overlay lingers briefly on
// return, because a spinner that simply vanishes leaves "it worked" and "it never
// restarted" indistinguishable.

const RestartOverlay = ({ restarting, justCameBack, failed, className }: RestartOverlayProps) => {
  const visible = restarting || justCameBack || Boolean(failed);
  if (!visible) return null;

  return (
    <div
      className={cn(
        "fixed inset-0 z-50 flex items-center justify-center bg-background/70 backdrop-blur-[2px]",
        // Transparent to pointer events ONLY once the app is usable again, so a stray
        // click during the confirmation moment cannot land on a control behind it.
        restarting || failed ? "cursor-progress" : "pointer-events-none",
        className,
      )}
      // A modal status: it takes the screen, and a screen reader should announce it
      // without waiting for focus to move.
      role="alertdialog"
      aria-modal="true"
      aria-live="polite"
      aria-label={restarting ? "Loomarr is restarting" : "Loomarr is back"}
    >
      <div className="flex max-w-sm items-center gap-3 rounded-lg border border-border bg-card p-4 shadow-lg">
        {restarting ? (
          <Loader2 className="size-5 shrink-0 animate-spin text-signal" aria-hidden />
        ) : failed ? null : (
          <Check className="size-5 shrink-0 text-lock" aria-hidden />
        )}
        <div className="min-w-0">
          <p className="font-medium text-sm">
            {restarting ? "Restarting Loomarr" : failed ? "Loomarr hasn't come back" : "Loomarr is back"}
          </p>
          <p className="mt-0.5 text-muted-foreground text-sm leading-snug">
            {restarting
              ? "This takes a few seconds. The app will pick up where it left off."
              : (failed ?? "Everything is back to normal.")}
          </p>
        </div>
      </div>
    </div>
  );
};

export { RestartOverlay };
