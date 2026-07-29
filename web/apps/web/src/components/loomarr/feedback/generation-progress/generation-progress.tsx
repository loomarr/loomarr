import type { SuggestionPhase } from "@loomarr/core";
import { Check, Loader2, X } from "lucide-react";
import { cn } from "@/lib";
import type { GenerationProgressProps } from "./generation-progress.type";

// GenerationProgress — the SSE suggester stepper (§3), driven by `suggestion` frames
// (searching → reasoning → scoring → done, or failed). The in-flight step carries
// the one sanctioned >300ms motion — the suggester shimmer (§2.4) in the `suggest`
// AI color (§2.1) — and stills under reduced-motion. Done steps lock green (§2.4);
// failed reads onair with the problem text (§6), never a raw stack.
const STEPS: { phase: SuggestionPhase; label: string; active: string }[] = [
  { phase: "reasoning", label: "Pick titles that fit", active: "Picking titles that fit" },
  { phase: "searching", label: "Search the library", active: "Searching the library" },
  { phase: "scoring", label: "Score the lineup", active: "Scoring the lineup" },
];

// Reasoning leads because that is the order the work happens in: the model thinks first,
// and only then asks for titles. The list used to open with "Search the library", which
// paired with a phase that was reported once up front to make every slow run look like a
// slow search. The slow part is the thinking.

// Seconds before the elapsed time appears. A quick run should not be narrated; a slow one
// should say something, because silence is what makes a wait feel broken.
const SHOW_ELAPSED_AFTER_S = 3;

// The small grey note beside the active step: which pass we are on, how long it has been
// going, or both. Plain words rather than jargon, because "round" is our internal name for
// a tool-loop iteration and means nothing to the person waiting.
const detail = (round: number | undefined, seconds: number | undefined): string => {
  const parts: string[] = [];
  // Pass 1 is the ordinary case and saying so adds nothing; from pass 2 the number is the
  // reassurance that repetition is the model working, not the UI stuck.
  if (round !== undefined && round > 1) parts.push(`pass ${round}`);
  if (seconds !== undefined) parts.push(`${seconds}s`);
  return parts.join(" · ");
};

const GenerationProgress = ({ phase, round, elapsedSeconds, error, className }: GenerationProgressProps) => {
  const failed = phase === "failed";
  const complete = phase === "done";
  const activeIdx = STEPS.findIndex((s) => s.phase === phase);
  // Only "scoring" (the last step, and the one that runs outside the tool loop) implies
  // the earlier steps are finished. The first two REPEAT — the model thinks, searches,
  // then thinks again — so an index-based "everything before the active step is done"
  // rule would tick reasoning off the moment a search began, and then un-tick it on the
  // next pass. A step is only ever done when the run has genuinely moved past the loop.
  const pastLoop = complete || phase === "scoring";
  const showElapsed = elapsedSeconds !== undefined && elapsedSeconds >= SHOW_ELAPSED_AFTER_S;
  const activeDetail = detail(round, showElapsed ? elapsedSeconds : undefined);
  const statusOf = (i: number): "done" | "active" | "todo" => {
    if (complete) return "done";
    if (failed) return "todo";
    if (i === activeIdx) return "active";
    if (pastLoop && i < activeIdx) return "done";
    return "todo";
  };
  return (
    <div className={cn("flex flex-col gap-2.5", className)}>
      <ol className="flex flex-col gap-2.5" aria-label="Generation progress">
        {STEPS.map((step, i) => {
          const status = statusOf(i);
          return (
            <li key={step.phase} className="flex items-center gap-3">
              <span className="flex size-5 items-center justify-center" aria-hidden>
                {status === "done" ? (
                  <Check className="size-4 text-lock" />
                ) : status === "active" ? (
                  <Loader2 className="size-4 animate-spin text-suggest-300" />
                ) : (
                  <span className="size-2 rounded-full bg-static-700" />
                )}
              </span>
              <span
                className={cn(
                  "text-sm",
                  status === "done" && "text-static-400",
                  status === "active" && "text-suggest-300 motion-safe:animate-pulse",
                  status === "todo" && "text-static-400",
                )}
              >
                {status === "active" ? `${step.active}…` : step.label}
              </span>
              {status === "active" && activeDetail !== "" && (
                // Tabular numerals so a ticking second count doesn't reflow the row.
                <span className="text-static-400 text-xs tabular-nums">{activeDetail}</span>
              )}
            </li>
          );
        })}
      </ol>
      {failed && (
        <p className="flex items-center gap-3 text-onair-300 text-sm" role="alert">
          <span className="flex size-5 items-center justify-center">
            <X className="size-4" aria-hidden />
          </span>
          {error ?? "Generation failed. Try again."}
        </p>
      )}
    </div>
  );
};

export { GenerationProgress };
