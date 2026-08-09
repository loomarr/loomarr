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
//
// ⚠ **THIS IS A STATUS REGION, NOT A DIALOG — and it used to claim otherwise.** It carried
// `role="alertdialog" aria-modal="true"` on a plain `fixed inset-0` div, with none of the
// behaviour either attribute promises: no portal, no focus trap, no focus restore, no scroll
// lock, no inert background. A screen reader was told the rest of the page was inert; Tab walked
// straight into it.
//
// V50b was going to make the claim true by rebuilding this on a real AlertDialog. It should not,
// for two reasons that only surfaced on reading it: `alertdialog` is defined for interrupting with
// a REQUIRED RESPONSE and requires a focusable element, and this overlay has no interactive
// content in any of its three states; and the "came back" state is deliberately
// `pointer-events-none` so a lingering confirmation cannot swallow a click on the app behind it,
// which a modal would forbid. Dropping the false attributes is the honest fix — a real modal would
// have satisfied the audit and broken the interaction.
//
// `alert` (assertive) when the restart FAILED, because that is an interruption worth one; `status`
// (polite) otherwise, so a routine restart does not talk over whatever the operator is reading.
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
      // It takes the screen, and a screen reader should announce it without waiting for focus to
      // move — which is what a live region does and what `aria-modal` never did here.
      //
      // ⚠ NO `aria-label`. The old one restated the heading below it, and on a live region a name
      // is the wrong tool: what gets announced is the region's CONTENTS, so the label was at best
      // redundant and at worst competing with the message it duplicated. Biome flagged it too —
      // the role is computed here, so the rule cannot narrow it and falls back to the div's
      // implicit role, which supports no label at all.
      role={failed ? "alert" : "status"}
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
