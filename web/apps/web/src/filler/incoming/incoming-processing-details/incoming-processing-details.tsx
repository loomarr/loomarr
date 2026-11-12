import type { IncomingProcessingDTO } from "@loomarr/api/models/incomingProcessingDTO";
import type { IncomingProcessingStageDTO } from "@loomarr/api/models/incomingProcessingStageDTO";
import { formatRelative, formatUntil } from "@loomarr/core/format";
import { Action, ProgressTrack, Stack, Text } from "@loomarr/design-system";
import { useState } from "react";

const visibleByDefault = (stage: IncomingProcessingStageDTO) => stage.outcome !== "not_needed";

const CurrentStageProgress = ({ stage }: { stage: IncomingProcessingStageDTO }) => {
  if (stage.outcome !== "in_progress") return null;
  const measured = stage.progress != null;
  const label = measured ? `Current step: ${stage.progress}%` : "Current step in progress";
  return (
    <div className="mt-3 space-y-1.5">
      <Stack align="center" direction="row" gap="control" justify="between">
        <Text as="span" textRole="hint" tone="secondary">
          Current step
        </Text>
        <Text as="span" textRole="hint" tone="primary">
          {measured ? `${stage.progress}%` : "Working…"}
        </Text>
      </Stack>
      <ProgressTrack percent={stage.progress} label={label} tone="tune" />
    </div>
  );
};

const ProcessingStage = ({ stage, timeZone }: { stage: IncomingProcessingStageDTO; timeZone?: string }) => (
  <li className="border-border relative min-w-0 border-l pb-5 pl-4 last:pb-0">
    <span
      aria-hidden
      className="border-background bg-muted-foreground absolute top-1.5 -left-[0.3125rem] size-2.5 rounded-full border-2"
    />
    <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <Text as="span" textRole="cardLabel">
        {stage.label}
      </Text>
      <Text as="span" textRole="hint" tone="secondary">
        {stage.outcomeLabel}
      </Text>
    </div>
    <Text as="p" className="mt-1 break-words" textRole="hint" tone="secondary">
      {stage.note}
    </Text>
    <time
      dateTime={stage.at}
      title={new Date(stage.at).toLocaleString(undefined, { timeZone })}
      className="mt-1 block"
    >
      <Text as="span" textRole="hint" tone="secondary">
        {formatRelative(stage.at)}
      </Text>
    </time>
    <CurrentStageProgress stage={stage} />
  </li>
);

// `timeZone` is the household's, so a step's full date on hover reads as the Guide tells times;
// undefined is the device's own.
const IncomingProcessingDetails = ({
  processing,
  timeZone,
}: {
  processing: IncomingProcessingDTO;
  timeZone?: string;
}) => {
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
    <details className="border-border rounded-lg border p-4">
      <Text as="summary" className="cursor-pointer" textRole="cardLabel">
        Processing details
      </Text>
      <div className="mt-4 space-y-4">
        {processing.attempts ? (
          <Text as="p" textRole="compact" tone="secondary">
            This step was tried {processing.attempts} {processing.attempts === 1 ? "time" : "times"}.
          </Text>
        ) : null}
        {processing.nextTryAt ? (
          <Text as="p" textRole="compact" tone="secondary">
            Next try {formatUntil(processing.nextTryAt)}
          </Text>
        ) : null}
        {stages.length ? (
          <ol className="pl-1">
            {keyedStages.map(({ key, stage }) => (
              // The ordered projection may contain identical stored occurrences. Its array
              // occurrence is part of the identity so content-derived keys do not collapse it.
              <ProcessingStage key={key} stage={stage} timeZone={timeZone} />
            ))}
          </ol>
        ) : (
          <Text as="p" textRole="compact" tone="secondary">
            No processing steps recorded yet.
          </Text>
        )}
        {skipped ? (
          <Stack direction="row">
            <Action onPress={() => setShowSkipped((open) => !open)} variant="ghost">
              {showSkipped
                ? "Hide skipped steps"
                : `Show ${skipped === 1 ? "skipped step" : `${skipped} skipped steps`}`}
            </Action>
          </Stack>
        ) : null}
      </div>
    </details>
  );
};

export { IncomingProcessingDetails };
