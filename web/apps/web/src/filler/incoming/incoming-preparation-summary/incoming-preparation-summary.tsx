import type { IncomingPreparationDTO } from "@loomarr/api/models/incomingPreparationDTO";
import { ProgressTrack, Stack, Text } from "@loomarr/design-system";

const readyRangeLabel = (lowerSeconds: number, upperSeconds: number): string => {
  if (upperSeconds < 60) return "Less than a minute";
  if (upperSeconds < 3600) {
    const lower = Math.max(1, Math.ceil(lowerSeconds / 60));
    const upper = Math.max(lower, Math.ceil(upperSeconds / 60));
    return lower === upper
      ? `About ${lower} ${lower === 1 ? "minute" : "minutes"}`
      : `About ${lower}–${upper} minutes`;
  }
  const lower = Math.max(1, Math.floor(lowerSeconds / 3600));
  const upper = Math.max(lower, Math.ceil(upperSeconds / 3600));
  return lower === upper
    ? `About ${lower} ${lower === 1 ? "hour" : "hours"}`
    : `About ${lower}–${upper} hours`;
};

const copy = (preparation: IncomingPreparationDTO) => {
  switch (preparation.state) {
    case "ready":
      return { title: "Ready", detail: "Finished preparing this clip." };
    case "waiting":
      return { title: "Waiting for your help", detail: "Loomarr will continue after your choice." };
    case "retrying":
      return { title: "Trying again", detail: "Loomarr will continue automatically." };
    case "restarted":
      return { title: "Started again", detail: "Preparing this clip from the requested step." };
    case "estimated":
      return {
        title: "Getting ready",
        detail: preparation.readyIn
          ? readyRangeLabel(preparation.readyIn.lowerSeconds, preparation.readyIn.upperSeconds)
          : "Estimating time remaining…",
      };
    case "estimating":
      return { title: "Getting ready", detail: "Estimating time remaining…" };
    case "working":
      return { title: "Getting ready", detail: "Loomarr is working in the background." };
    default:
      return {
        title: "Preparation progress unavailable",
        detail: "This clip started before progress tracking was available.",
      };
  }
};

const IncomingPreparationSummary = ({ preparation }: { preparation: IncomingPreparationDTO }) => {
  const message = copy(preparation);
  const showBar =
    preparation.percent != null && !["waiting", "retrying", "unavailable"].includes(preparation.state);

  return (
    <section
      aria-labelledby="incoming-preparation-heading"
      className="bg-muted/30 space-y-2.5 rounded-lg p-4"
    >
      <Stack align="baseline" direction="row" gap="control" justify="between">
        <div className="min-w-0">
          <Text as="h3" id="incoming-preparation-heading" textRole="cardLabel">
            {message.title}
          </Text>
          <Text as="p" className="mt-0.5" textRole="hint" tone="secondary">
            {message.detail}
          </Text>
        </div>
        {preparation.percent != null ? (
          <Text as="span" className="shrink-0 tabular-nums" textRole="hint" tone="secondary">
            {preparation.percent}%
          </Text>
        ) : null}
      </Stack>
      {showBar ? <ProgressTrack percent={preparation.percent} label="Clip preparation" tone="tune" /> : null}
    </section>
  );
};

export { IncomingPreparationSummary, readyRangeLabel };
