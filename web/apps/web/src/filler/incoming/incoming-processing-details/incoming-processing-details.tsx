import type { IncomingProcessingDTO } from "@loomarr/api/models/incomingProcessingDTO";
import type { IncomingProcessingStageDTO } from "@loomarr/api/models/incomingProcessingStageDTO";
import { formatRelative, formatUntil } from "@loomarr/core/format";
import { Action, Card, Details, ProgressTrack, Stack, Text } from "@loomarr/design-system";
import { useState } from "react";

const visibleByDefault = (stage: IncomingProcessingStageDTO) => stage.outcome !== "not_needed";

const CurrentStageProgress = ({ stage }: { stage: IncomingProcessingStageDTO }) => {
  if (stage.outcome !== "in_progress") return null;
  const measured = stage.progress != null;
  const label = measured ? `Current step: ${stage.progress}%` : "Current step in progress";
  return (
    <Stack gap="1.5" marginTop={12}>
      <Stack align="center" direction="row" gap="control" justify="between">
        <Text as="span" textRole="hint" tone="secondary">
          Current step
        </Text>
        <Text as="span" textRole="hint" tone="primary">
          {measured ? `${stage.progress}%` : "Working…"}
        </Text>
      </Stack>
      <ProgressTrack percent={stage.progress} label={label} tone="tune" />
    </Stack>
  );
};

const ProcessingStage = ({
  last,
  stage,
  timeZone,
}: {
  last: boolean;
  stage: IncomingProcessingStageDTO;
  timeZone?: string;
}) => (
  <Stack
    borderColor="borderDecorative"
    borderLeftWidth={1}
    minWidth={0}
    paddingBottom={last ? 0 : 20}
    paddingLeft={16}
    position="relative"
    render="li"
  >
    <Stack
      aria-hidden
      backgroundColor="contentSecondary"
      borderColor="surfaceCanvas"
      borderRadius={999}
      borderWidth={2}
      height={10}
      left={-5}
      position="absolute"
      top={6}
      width={10}
    />
    <Stack align="baseline" direction="row" flexShrink={1} gap="3" justify="between" minWidth={0} wrap>
      <Text as="span" textRole="cardLabel">
        {stage.label}
      </Text>
      <Text as="span" textRole="hint" tone="secondary">
        {stage.outcomeLabel}
      </Text>
    </Stack>
    <Stack marginTop={4}>
      <Text as="p" overflowWrap="break-word" textRole="hint" tone="secondary">
        {stage.note}
      </Text>
    </Stack>
    <Stack marginTop={4}>
      <time dateTime={stage.at} title={new Date(stage.at).toLocaleString(undefined, { timeZone })}>
        <Text as="span" textRole="hint" tone="secondary">
          {formatRelative(stage.at)}
        </Text>
      </time>
    </Stack>
    <CurrentStageProgress stage={stage} />
  </Stack>
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
    <Card>
      <Stack padding={16}>
        <Details summary="Processing details" textRole="cardLabel">
          <Stack gap="4" marginTop={16}>
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
              <Stack paddingLeft={4} render="ol">
                {keyedStages.map(({ key, stage }, index) => (
                  // The ordered projection may contain identical stored occurrences. Its array
                  // occurrence is part of the identity so content-derived keys do not collapse it.
                  <ProcessingStage
                    key={key}
                    last={index === keyedStages.length - 1}
                    stage={stage}
                    timeZone={timeZone}
                  />
                ))}
              </Stack>
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
          </Stack>
        </Details>
      </Stack>
    </Card>
  );
};

export { IncomingProcessingDetails };
