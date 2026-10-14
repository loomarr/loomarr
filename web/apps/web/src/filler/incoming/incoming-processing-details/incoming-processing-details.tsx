import type { IncomingProcessingDTO } from "@loomarr/api/models/incomingProcessingDTO";
import type { IncomingProcessingStageDTO } from "@loomarr/api/models/incomingProcessingStageDTO";
import { formatRelative, formatUntil } from "@loomarr/core/format";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";

const visibleByDefault = (stage: IncomingProcessingStageDTO) => stage.outcome !== "not_needed";

const CurrentStageProgress = ({ stage }: { stage: IncomingProcessingStageDTO }) => {
  if (stage.outcome !== "in_progress") return null;
  const measured = stage.progress != null;
  const label = measured ? `Current step: ${stage.progress}%` : "Current step in progress";
  return (
    <div className="mt-3 space-y-1.5">
      <div className="flex items-center justify-between gap-3 text-xs">
        <span className="text-muted-foreground">Current step</span>
        <span>{measured ? `${stage.progress}%` : "Working…"}</span>
      </div>
      <Progress value={stage.progress} label={label} />
    </div>
  );
};

const ProcessingStage = ({ stage }: { stage: IncomingProcessingStageDTO }) => (
  <li className="relative min-w-0 border-border border-l pb-5 pl-4 last:pb-0">
    <span
      aria-hidden
      className="absolute top-1.5 -left-[0.3125rem] size-2.5 rounded-full border-2 border-background bg-muted-foreground"
    />
    <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <span className="font-medium">{stage.label}</span>
      <span className="text-muted-foreground text-xs">{stage.outcomeLabel}</span>
    </div>
    <p className="mt-1 break-words text-muted-foreground text-xs">{stage.note}</p>
    <time
      dateTime={stage.at}
      title={new Date(stage.at).toLocaleString()}
      className="mt-1 block text-muted-foreground text-xs"
    >
      {formatRelative(stage.at)}
    </time>
    <CurrentStageProgress stage={stage} />
  </li>
);

const IncomingProcessingDetails = ({ processing }: { processing: IncomingProcessingDTO }) => {
  const [showSkipped, setShowSkipped] = useState(false);
  const skipped = processing.stages.filter((stage) => stage.outcome === "not_needed").length;
  const stages = showSkipped ? processing.stages : processing.stages.filter(visibleByDefault);
  const occurrences = new Map<string, number>();
  const keyedStages = stages.map((stage) => {
    const identity = JSON.stringify(stage);
    const occurrence = occurrences.get(identity) ?? 0;
    occurrences.set(identity, occurrence + 1);
    return { key: JSON.stringify([identity, occurrence]), stage };
  });

  return (
    <details className="rounded-lg border border-border p-4 text-sm">
      <summary className="cursor-pointer font-medium">Processing details</summary>
      <div className="mt-4 space-y-4">
        {processing.attempts ? (
          <p className="text-muted-foreground">
            This step was tried {processing.attempts} {processing.attempts === 1 ? "time" : "times"}.
          </p>
        ) : null}
        {processing.nextTryAt ? (
          <p className="text-muted-foreground">Next try {formatUntil(processing.nextTryAt)}</p>
        ) : null}
        {stages.length ? (
          <ol className="pl-1">
            {keyedStages.map(({ key, stage }) => (
              // The ordered projection may contain identical stored occurrences. Its array
              // occurrence is part of the identity so content-derived keys do not collapse it.
              <ProcessingStage key={key} stage={stage} />
            ))}
          </ol>
        ) : (
          <p className="text-muted-foreground">No processing steps recorded yet.</p>
        )}
        {skipped ? (
          <Button type="button" variant="ghost" size="sm" onClick={() => setShowSkipped((open) => !open)}>
            {showSkipped
              ? "Hide skipped steps"
              : `Show ${skipped === 1 ? "skipped step" : `${skipped} skipped steps`}`}
          </Button>
        ) : null}
        {processing.diagnosticsHref ? (
          <Button render={<a href={processing.diagnosticsHref} />} variant="outline" size="sm">
            View diagnostics
          </Button>
        ) : null}
      </div>
    </details>
  );
};

export { IncomingProcessingDetails };
