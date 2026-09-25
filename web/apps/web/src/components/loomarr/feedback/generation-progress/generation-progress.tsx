import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { pickCount, progressLine } from "@/components/loomarr/feedback/progress-line";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { GenerationProgressProps } from "./generation-progress.type";

const LABELS: Partial<Record<GenerationProgressProps["phase"], string>> = {
  reasoning: "Understanding your channel…",
  searching: "Looking for matching titles…",
  scoring: "Preparing your channel…",
  done: "Your channel is ready to review.",
};

// Whole seconds since the server started the run, so a reload or a second device shows the same
// number the first tab did. Clamped: a browser clock behind the server's must not read negative.
const useSecondsSince = (startedAt: string | undefined): number | undefined => {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!startedAt) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [startedAt]);
  if (!startedAt) return undefined;
  return Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));
};

// A streamed title reads like a row of the lineup it is about to become: name, year, and the
// same state wording the review uses.
const PickRow = ({ pick }: { pick: ProposalJourneyProgressDTO["picks"][number] }) => (
  <li className="flex flex-wrap items-center gap-x-2 gap-y-1 border-border border-b py-2 last:border-b-0">
    <span className="font-medium text-sm">{pick.name}</span>
    {pick.year ? <span className="text-muted-foreground text-xs">{pick.year}</span> : null}
    <Badge variant={pick.inLibrary ? "lock" : "tune"}>
      {pick.inLibrary ? "In your library" : "Will be added"}
    </Badge>
  </li>
);

// A generation is a loop, not a trustworthy checklist. Show the user's current outcome in
// one stable place — the stage line — and let the titles stream in beneath it as the model
// chooses them; the terminal failure surface owns recovery when the loop ends.
//
// Only the stage line is a live region. The clock ticks every second and the list grows, and
// announcing either would make assistive tech chatter through a long wait.
const GenerationProgress = ({ phase, elapsedSeconds, progress, className }: GenerationProgressProps) => {
  const serverSeconds = useSecondsSince(progress?.startedAt);
  // An explicit elapsedSeconds wins: it is what a caller with its own clock (and a story) pins.
  const seconds = elapsedSeconds ?? serverSeconds;
  if (phase === "failed") return null;
  const complete = phase === "done";
  const line = progress ? progressLine(progress) : (LABELS[phase] ?? LABELS.reasoning);
  const count = progress ? pickCount(progress) : undefined;
  return (
    <div className={cn("flex items-start gap-3 rounded-lg border border-border bg-muted/35 p-4", className)}>
      {!complete && (
        <Loader2
          className="mt-0.5 size-4 shrink-0 animate-spin text-suggest-300 motion-reduce:animate-none"
          aria-hidden
        />
      )}
      <div className="min-w-0 flex-1">
        <p role="status" aria-live="polite" className="font-medium text-sm">
          {line}
        </p>
        {progress && progress.picks.length > 0 && (
          <>
            <ul aria-label="Titles chosen so far" className="mt-2 flex flex-col">
              {progress.picks.map((pick) => (
                <PickRow key={pick.key} pick={pick} />
              ))}
            </ul>
            <p className="mt-1 text-muted-foreground text-xs">
              {count}
              {seconds !== undefined ? ` · ${seconds} s` : ""}
            </p>
          </>
        )}
        {!complete && seconds !== undefined && seconds >= 10 && !(progress && progress.picks.length > 0) && (
          <p className="mt-1 text-muted-foreground text-sm">
            This can take a little while. You can leave this page open.
          </p>
        )}
      </div>
    </div>
  );
};

export { GenerationProgress };
