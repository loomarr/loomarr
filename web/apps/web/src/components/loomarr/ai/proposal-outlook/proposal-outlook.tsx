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

const describedDuration = (milliseconds: number, lowerBound = false) =>
  lowerBound ? `at least ${duration(milliseconds, true)}` : duration(milliseconds);

const sentenceCase = (value: string) => `${value.charAt(0).toUpperCase()}${value.slice(1)}`;

const ProposalOutlookDiagnostics = ({ assessment: value }: { assessment: Assessment }) => (
  <div className="flex flex-col gap-2 text-muted-foreground">
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
      {(
        [
          ["Requested or kept", value.mix.core],
          ["Related suggestions", value.mix.adjacent],
          ["Other suggestions", value.mix.discovery],
          ["Uncategorized", value.mix.unknown],
        ] as const
      )
        .filter(([, count]) => count > 0)
        .map(([label, count]) => (
          <div key={label} className="contents">
            <dt>{label}</dt>
            <dd>{count}</dd>
          </div>
        ))}
      <dt>Seasons represented</dt>
      <dd>{value.seasons}</dd>
    </dl>
    <p>
      This estimate reflects the current title list and schedule settings. Missing titles and breaks do not
      add playing time.
    </p>
    {value.relaxations.length > 0 && (
      <p>
        Loomarr loosened {value.relaxations.length} scheduling{" "}
        {value.relaxations.length === 1 ? "rule" : "rules"} to build this preview. Your audience and title
        limits still apply.
      </p>
    )}
    <p>
      Checked{" "}
      {new Date(value.observedAt).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      })}
      . Changes to your library or schedule can update this result.
    </p>
  </div>
);

const ProposalOutlookDetails = ({
  assessment: value,
  showDiagnostics = true,
}: {
  assessment: Assessment;
  showDiagnostics?: boolean;
}) => (
  <section aria-label="What can play now" className="flex flex-col gap-2 text-muted-foreground">
    <h3 className="font-medium text-foreground">What can play now</h3>
    <p>
      {value.programs > 0
        ? `${value.programs} playable ${value.programs === 1 ? "episode or movie" : "episodes or movies"} from ${value.scheduledTitles} ${value.scheduledTitles === 1 ? "title" : "titles"}.`
        : value.state === "uncertain"
          ? "We haven't confirmed what's ready to play yet."
          : "No episodes or movies are ready to play yet."}
    </p>
    {value.missingAcquisitions > 0 && (
      <p>
        {value.missingAcquisitions}{" "}
        {value.missingAcquisitions === 1 ? "title still needs" : "titles still need"} adding and{" "}
        {value.missingAcquisitions === 1 ? "isn't" : "aren't"} included in the estimate yet.
      </p>
    )}
    {value.missingLibrary > 0 && (
      <p>
        {value.missingLibrary} library {value.missingLibrary === 1 ? "title isn't" : "titles aren't"} ready to
        play and {value.missingLibrary === 1 ? "isn't" : "aren't"} included in the estimate.
      </p>
    )}
    {value.unknownTitles > 0 && (
      <p>
        Availability hasn't been confirmed for {value.unknownTitles}{" "}
        {value.unknownTitles === 1 ? "title" : "titles"}.{" "}
        {value.unknownTitles === 1 ? "It isn't" : "They aren't"} included in the estimate.
      </p>
    )}
    {value.windowLimited && <p>This is a partial preview; more from your library may fit.</p>}
    {showDiagnostics && (
      <details className="mt-1">
        <summary className="w-fit cursor-pointer">How this estimate works</summary>
        <div className="mt-3">
          <ProposalOutlookDiagnostics assessment={value} />
        </div>
      </details>
    )}
  </section>
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

export { ProposalOutlook, ProposalOutlookDetails, ProposalOutlookDiagnostics };
