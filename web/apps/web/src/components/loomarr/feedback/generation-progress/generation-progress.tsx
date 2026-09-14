import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { GenerationProgressProps } from "./generation-progress.type";

const LABELS: Partial<Record<GenerationProgressProps["phase"], string>> = {
  reasoning: "Understanding your channel…",
  searching: "Looking for matching titles…",
  scoring: "Preparing your channel…",
  done: "Your channel is ready to review.",
};

// A generation is a loop, not a trustworthy checklist. Show the user's current outcome in
// one stable place and let the terminal failure surface own recovery when the loop ends.
const GenerationProgress = ({ phase, elapsedSeconds, className }: GenerationProgressProps) => {
  if (phase === "failed") return null;
  const complete = phase === "done";
  return (
    <div
      className={cn("flex items-start gap-3 rounded-lg border border-border bg-muted/35 p-4", className)}
      aria-live="polite"
    >
      {!complete && <Loader2 className="mt-0.5 size-4 shrink-0 animate-spin text-suggest-300" aria-hidden />}
      <div>
        <p className="font-medium text-sm">{LABELS[phase] ?? LABELS.reasoning}</p>
        {!complete && elapsedSeconds !== undefined && elapsedSeconds >= 10 && (
          <p className="mt-1 text-muted-foreground text-sm">
            This can take a little while. You can leave this page open.
          </p>
        )}
      </div>
    </div>
  );
};

export { GenerationProgress };
