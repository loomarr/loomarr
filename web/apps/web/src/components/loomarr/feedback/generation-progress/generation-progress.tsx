import type { SuggestionPhase } from "@loomarr/core/events";
import { Check, Loader2, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { GenerationProgressProps } from "./generation-progress.type";

// GenerationProgress — the SSE suggester stepper (§3), driven by `suggestion` frames.
// The in-flight step carries the one sanctioned >300ms motion — the suggester shimmer
// (§2.4) in the `suggest` AI color (§2.1) — and stills under reduced-motion. Done steps
// lock green (§2.4); failed reads onair with the problem text (§6), never a raw stack.
//
// ⚠ **Two steps, not three, because the backend emits a LOOP, not a line.** A run reports
// `reasoning → searching → reasoning → searching → … → scoring → done`: the model thinks,
// asks for titles, thinks again, for as many passes as it needs (a real run was measured at
// six reasoning/searching pairs). Mapping `reasoning` and `searching` to two SEPARATE steps
// made the active spinner march to step 2 and then jump BACKWARD to step 1 on the next pass,
// over and over — a checklist walking backwards, which reads as broken. They are one piece of
// work ("find the titles") alternating internally, so they share ONE step; `round` (shown as
// "pass N") is what conveys the loop advancing. Only `scoring` genuinely follows.
const STEPS: { phases: SuggestionPhase[]; label: string; active: string }[] = [
  { phases: ["reasoning", "searching"], label: "Find the titles", active: "Finding the titles" },
  { phases: ["scoring"], label: "Score the lineup", active: "Scoring the lineup" },
];

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
  const activeIdx = phase ? STEPS.findIndex((s) => s.phases.includes(phase)) : -1;
  const showElapsed = elapsedSeconds !== undefined && elapsedSeconds >= SHOW_ELAPSED_AFTER_S;
  const activeDetail = detail(round, showElapsed ? elapsedSeconds : undefined);
  // With the loop folded into one step, "everything before the active step is done" is now
  // honest: the only backward step left is scoring→find, which never happens (the run only
  // moves forward across these two). A completed run marks both done.
  const statusOf = (i: number): "done" | "active" | "todo" => {
    if (complete) return "done";
    if (failed) return "todo";
    if (i === activeIdx) return "active";
    if (i < activeIdx) return "done";
    return "todo";
  };
  return (
    <div className={cn("flex flex-col gap-2.5", className)}>
      <ol className="flex flex-col gap-2.5" aria-label="Generation progress">
        {STEPS.map((step, i) => {
          const status = statusOf(i);
          return (
            <li key={step.label} className="flex items-center gap-3">
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
