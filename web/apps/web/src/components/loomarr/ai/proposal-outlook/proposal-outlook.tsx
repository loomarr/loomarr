import type { Assessment } from "@loomarr/api/models/assessment";

interface ProposalOutlookProps {
  assessment?: Assessment;
  pending?: boolean;
  showDetails?: boolean;
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
  if (known === 0) return null;
  if (mix.core > known / 2 && mix.adjacent + mix.discovery > 0)
    return "Mostly requested picks, with a few related or discovery choices.";
  if (mix.core === known) return "Built around your requested or retained picks.";
  if (mix.adjacent > known / 2) return "Mostly related picks that fit alongside your request.";
  if (mix.discovery > known / 2) return "Mostly new finds that match your request.";
  const roles = [
    mix.core > 0 && "requested",
    mix.adjacent > 0 && "related",
    mix.discovery > 0 && "discovery",
  ].filter(Boolean);
  return `A mix of ${roles.join(" and ")} picks.`;
};

const describedDuration = (milliseconds: number, lowerBound = false) =>
  lowerBound ? `at least ${duration(milliseconds, true)}` : duration(milliseconds);

const sentenceCase = (value: string) => `${value.charAt(0).toUpperCase()}${value.slice(1)}`;

const ProposalOutlookDetails = ({ assessment: value }: { assessment: Assessment }) => (
  <div className="flex flex-col gap-2 text-muted-foreground">
    <p>
      {value.scheduledTitles} of {value.titles} titles currently produce {value.programs}{" "}
      {value.programs === 1 ? "unique program" : "unique programs"} across {value.seasons}{" "}
      {value.seasons === 1 ? "numbered season" : "numbered seasons"}.
    </p>
    {mixCopy(value.mix) && (
      <p>
        {mixCopy(value.mix)}
        {value.mix.unknown > 0 && value.mix.core + value.mix.adjacent + value.mix.discovery > 0
          ? " Some picks have no recorded role."
          : ""}
      </p>
    )}
    <p>
      Core {value.mix.core} · Adjacent {value.mix.adjacent} · Discovery {value.mix.discovery} · Unclassified{" "}
      {value.mix.unknown}.
    </p>
    <p>
      Core means requested or retained. Adjacent choices come from recorded recommendations, and discovery
      choices are other grounded matches.
    </p>
    <p>
      The estimate uses this exact lineup, {value.ordering || "inherited"} ordering, separation, and active
      scheduling rules. Unavailable media and breaks add no programming time.
    </p>
    {value.missingAcquisitions + value.missingLibrary + value.unknownTitles > 0 && (
      <p>
        {value.missingAcquisitions} acquisitions, {value.missingLibrary} other library titles, and{" "}
        {value.unknownTitles} incomplete observations are excluded from the current estimate.
      </p>
    )}
    {value.windowLimited && (
      <p>The scheduling window is limited, so additional library programs may extend this estimate.</p>
    )}
    {value.relaxations.length > 0 && (
      <p>
        The scheduler used {value.relaxations.length} recorded policy{" "}
        {value.relaxations.length === 1 ? "relaxation" : "relaxations"}. Audience and scope filters remain
        enforced.
      </p>
    )}
    <p>
      Observed{" "}
      {new Date(value.observedAt).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      })}
      . Library and policy changes can alter the result.
    </p>
  </div>
);

const ProposalOutlook = ({
  assessment: value,
  pending = false,
  showDetails = true,
}: ProposalOutlookProps) => {
  if (pending)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        Checking this lineup against your library…
      </p>
    );
  if (!value)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        A schedule preview isn't available yet. You can still review the titles.
      </p>
    );
  const lowerBound = value.windowLimited || value.unknownTitles > 0;
  const repeatDuration =
    value.firstRepeatMs === null ? null : describedDuration(value.firstRepeatMs, lowerBound);
  const programmingDuration = describedDuration(value.uniqueRuntimeMs, lowerBound);
  const summary =
    value.programs === 0
      ? value.state === "uncertain"
        ? "A schedule estimate will appear when availability has been checked."
        : "A schedule estimate will appear as the selected titles are added."
      : repeatDuration !== null
        ? value.thin
          ? `Short lineup: ${repeatDuration} before repeats.`
          : `${sentenceCase(repeatDuration)} before this lineup repeats.`
        : value.thin
          ? `Short lineup: ${programmingDuration} of programming.`
          : `${sentenceCase(programmingDuration)} of programming.`;
  return (
    <section aria-label="Channel outlook" className="text-sm">
      <p className={value.thin ? "text-caution" : "text-muted-foreground"}>{summary}</p>
      {showDetails && (
        <details className="mt-2">
          <summary className="cursor-pointer text-muted-foreground">Schedule details</summary>
          <div className="mt-2">
            <ProposalOutlookDetails assessment={value} />
          </div>
        </details>
      )}
    </section>
  );
};

export { ProposalOutlook, ProposalOutlookDetails };
