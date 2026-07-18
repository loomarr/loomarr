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
  { phase: "searching", label: "Search the library", active: "Searching the library" },
  { phase: "reasoning", label: "Reason about fit", active: "Reasoning about fit" },
  { phase: "scoring", label: "Score the lineup", active: "Scoring the lineup" },
];

const GenerationProgress = ({ phase, error, className }: GenerationProgressProps) => {
  const failed = phase === "failed";
  const complete = phase === "done";
  const activeIdx = STEPS.findIndex((s) => s.phase === phase);
  const statusOf = (i: number): "done" | "active" | "todo" => {
    if (complete) return "done";
    if (failed) return "todo";
    if (i < activeIdx) return "done";
    if (i === activeIdx) return "active";
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
