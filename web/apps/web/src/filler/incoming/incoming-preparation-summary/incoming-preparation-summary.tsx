import type { IncomingPreparationDTO } from "@loomarr/api/models/incomingPreparationDTO";
import { Progress } from "@/components/ui/progress";

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
      <div className="flex items-baseline justify-between gap-3">
        <div className="min-w-0">
          <h3 id="incoming-preparation-heading" className="text-sm font-medium">
            {message.title}
          </h3>
          <p className="text-muted-foreground mt-0.5 text-xs">{message.detail}</p>
        </div>
        {preparation.percent != null ? (
          <span className="text-muted-foreground shrink-0 text-xs tabular-nums">{preparation.percent}%</span>
        ) : null}
      </div>
      {showBar ? <Progress value={preparation.percent} label="Clip preparation" /> : null}
    </section>
  );
};

export { IncomingPreparationSummary, readyRangeLabel };
