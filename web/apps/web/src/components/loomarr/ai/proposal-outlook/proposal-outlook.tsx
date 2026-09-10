import type { Assessment } from "@loomarr/api/models/assessment";

import { Button } from "@/components/ui/button";

interface ProposalOutlookProps {
  onAddVariety?: () => void;
  assessment?: Assessment;
  pending?: boolean;
}

const duration = (milliseconds: number, lowerBound = false) => {
  const minutes = lowerBound
    ? Math.floor(milliseconds / 60_000)
    : Math.max(1, Math.round(milliseconds / 60_000));
  const prefix = lowerBound ? "" : "about ";
  if (minutes < 60) return `${prefix}${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
  const hours = minutes / 60;
  const lower = Math.floor(hours);
  const upper = Math.ceil(hours);
  if (lowerBound || lower === upper) return `${prefix}${lower} ${lower === 1 ? "hour" : "hours"}`;
  return `about ${lower}–${upper} hours`;
};

const mixCopy = (mix: Assessment["mix"]) => {
  const known = mix.core + mix.adjacent + mix.discovery;
  if (known === 0) return "The original run has no recorded editorial mix evidence.";
  if (mix.core > known / 2 && mix.adjacent + mix.discovery > 0)
    return "Mostly requested picks, with a few related or discovery choices.";
  if (mix.core === known) return "Built around your requested or retained picks.";
  if (mix.adjacent > known / 2) return "Mostly related picks from the recommendation graph.";
  if (mix.discovery > known / 2) return "Discovery-led picks grounded in the catalog.";
  const roles = [
    mix.core > 0 && "requested",
    mix.adjacent > 0 && "related",
    mix.discovery > 0 && "discovery",
  ].filter(Boolean);
  return `A mix of ${roles.join(" and ")} picks.`;
};

const ProposalOutlook = ({ assessment: value, pending = false, onAddVariety }: ProposalOutlookProps) => {
  if (pending)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        Checking this lineup against your library…
      </p>
    );
  if (!value)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        Launch and fresh-programming estimates are unavailable. Review the picks before approving.
      </p>
    );
  const launch =
    value.state === "ready"
      ? value.missingAcquisitions + value.missingLibrary + value.unknownTitles > 0
        ? "Starts with available programming after approval"
        : "Starts now after approval"
      : value.state === "waiting"
        ? `Waiting on ${value.missingAcquisitions} ${value.missingAcquisitions === 1 ? "acquisition" : "acquisitions"}`
        : value.state === "uncertain"
          ? "Launch readiness is uncertain"
          : "No eligible programming can start yet";
  return (
    <section aria-label="Channel outlook" className="flex flex-col gap-2 text-sm">
      <p className="font-medium">{launch}</p>
      {value.state === "ready" && value.missingAcquisitions > 0 && (
        <p>
          {value.missingAcquisitions}{" "}
          {value.missingAcquisitions === 1 ? "acquisition still missing" : "acquisitions still missing"}.
        </p>
      )}
      {value.state === "ready" && value.missingLibrary + value.unknownTitles > 0 && (
        <p>Some titles still need available media or complete library observations.</p>
      )}
      {value.programs > 0 ? (
        <p>
          {value.windowLimited || value.unknownTitles > 0 ? "At least " : "This cycle has "}
          {duration(value.uniqueRuntimeMs, value.windowLimited || value.unknownTitles > 0)} of fresh
          programming.
          {value.firstRepeatMs !== null &&
            ` Its first repeat comes after ${duration(value.firstRepeatMs)} of program time.`}
        </p>
      ) : (
        <p>Fresh-programming time can be estimated when eligible media is available.</p>
      )}
      <p>
        {mixCopy(value.mix)}
        {value.mix.unknown > 0 && value.mix.core + value.mix.adjacent + value.mix.discovery > 0
          ? " Some picks have no recorded role."
          : ""}
      </p>
      {value.thin && (
        <div className="flex flex-col gap-2">
          <p role="status" className="text-caution">
            May feel repetitive. Add more variety to extend the fresh programming.
          </p>
          {onAddVariety && (
            <Button variant="outline" size="sm" className="w-fit" onClick={onAddVariety}>
              Add more variety
            </Button>
          )}
        </div>
      )}
      <details>
        <summary className="cursor-pointer text-muted-foreground">How we estimated this</summary>
        <div className="mt-2 flex flex-col gap-1 text-muted-foreground">
          <p>
            {value.scheduledTitles} of {value.titles} titles appear as eligible programming: {value.programs}{" "}
            unique programs across {value.seasons} numbered seasons.
          </p>
          <p>
            {value.missingAcquisitions} acquisitions are missing; {value.missingLibrary} other titles are
            missing; {value.unknownTitles} titles have incomplete library observations.
          </p>
          <p>
            Core {value.mix.core} · Adjacent {value.mix.adjacent} · Discovery {value.mix.discovery} ·
            Unclassified {value.mix.unknown}.
          </p>
          <p>
            Core means requested or retained, not inferred favorites. Adjacent requires recorded
            recommendations. Discovery means other grounded choices. Review additions and older missing
            evidence stay unclassified.
          </p>
          <p>
            Uses this exact lineup, {value.ordering || "inherited"} ordering, separation and active scheduling
            rules. Breaks and unavailable media add no fresh-programming time.
          </p>
          {value.windowLimited && (
            <p>
              The scheduling window is limited; additional library programs may extend the runway beyond this
              estimate.
            </p>
          )}
          {value.relaxations.length > 0 && (
            <p>
              The scheduler needed {value.relaxations.length} recorded policy relaxations to place this
              lineup. Audience and scope filters remain enforced.
            </p>
          )}
          <p>
            Observed{" "}
            {new Date(value.observedAt).toLocaleString(undefined, {
              dateStyle: "medium",
              timeStyle: "short",
            })}
            . Library and policy changes can alter the result. Starts now describes schedulable media after
            approval; playback preparation is separate.
          </p>
        </div>
      </details>
    </section>
  );
};

export { ProposalOutlook };
