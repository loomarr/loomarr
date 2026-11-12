import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import { pickCount, progressLine } from "@loomarr/core/requests";
import { ActivityIndicator, Badge, Stack } from "@loomarr/design-system";
import { useEffect, useState } from "react";
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
  <li className="border-border flex flex-wrap items-center gap-x-2 gap-y-1 border-b py-2 last:border-b-0">
    <span className="text-sm font-medium">{pick.name}</span>
    {pick.year ? <span className="text-muted-foreground text-xs">{pick.year}</span> : null}
    <Badge shape="square" tone={pick.inLibrary ? "lock" : "tune"}>
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
    <div className={cn("border-border bg-muted/35 flex items-start gap-3 rounded-lg border p-4", className)}>
      {!complete && (
        <Stack className="mt-0.5" direction="row" render="span">
          <ActivityIndicator decorative size="compact" tone="suggest" />
        </Stack>
      )}
      <div className="min-w-0 flex-1">
        <p role="status" aria-live="polite" className="text-sm font-medium">
          {line}
        </p>
        {progress && progress.picks.length > 0 && (
          <>
            <Stack aria-label="Titles chosen so far" className="mt-2" render="ul">
              {progress.picks.map((pick) => (
                <PickRow key={pick.key} pick={pick} />
              ))}
            </Stack>
            <p className="text-muted-foreground mt-1 text-xs">
              {count}
              {seconds !== undefined ? ` · ${seconds} s` : ""}
            </p>
          </>
        )}
        {!complete && seconds !== undefined && seconds >= 10 && !(progress && progress.picks.length > 0) && (
          <p className="text-muted-foreground mt-1 text-sm">
            This can take a little while. You can leave this page open.
          </p>
        )}
      </div>
    </div>
  );
};

export { GenerationProgress };
