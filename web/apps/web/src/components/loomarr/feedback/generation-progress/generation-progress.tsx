import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import { pickCount, progressLine } from "@loomarr/core/requests";
import { ActivityIndicator, Badge, Stack, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
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
const PickRow = ({ last, pick }: { last: boolean; pick: ProposalJourneyProgressDTO["picks"][number] }) => (
  <Stack
    align="center"
    borderBottomWidth={last ? 0 : 1}
    borderColor="$borderDecorative"
    columnGap="2"
    direction="row"
    flexShrink={0}
    paddingVertical={8}
    render="li"
    rowGap="1"
    wrap
  >
    <Text as="span" textRole="cardLabel">
      {pick.name}
    </Text>
    {pick.year ? (
      <Text as="span" textRole="hint" tone="secondary">
        {pick.year}
      </Text>
    ) : null}
    <Badge shape="square" tone={pick.inLibrary ? "lock" : "tune"}>
      {pick.inLibrary ? "In your library" : "Will be added"}
    </Badge>
  </Stack>
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
    <Stack
      align="start"
      backgroundColor="washElevated35"
      borderColor="$borderDecorative"
      borderRadius={8}
      borderWidth={1}
      className={className}
      direction="row"
      gap="control"
      padding={16}
    >
      {!complete && (
        <Stack direction="row" marginTop={2} render="span">
          <ActivityIndicator decorative size="compact" tone="suggest" />
        </Stack>
      )}
      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
        <div role="status" aria-live="polite">
          <Text as="p" textRole="cardLabel">
            {line}
          </Text>
        </div>
        {progress && progress.picks.length > 0 && (
          <>
            <Stack aria-label="Titles chosen so far" marginTop={8} render="ul">
              {progress.picks.map((pick, index) => (
                <PickRow key={pick.key} last={index === progress.picks.length - 1} pick={pick} />
              ))}
            </Stack>
            <Stack marginTop={4}>
              <Text as="p" textRole="hint" tone="secondary">
                {count}
                {seconds !== undefined ? ` · ${seconds} s` : ""}
              </Text>
            </Stack>
          </>
        )}
        {!complete && seconds !== undefined && seconds >= 10 && !(progress && progress.picks.length > 0) && (
          <Stack marginTop={4}>
            <Text as="p" textRole="compact" tone="secondary">
              This can take a little while. You can leave this page open.
            </Text>
          </Stack>
        )}
      </Stack>
    </Stack>
  );
};

export { GenerationProgress };
