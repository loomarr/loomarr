import type { Assessment } from "@loomarr/api/models/assessment";
import { Details, Grid, Stack, Text } from "@loomarr/design-system";
import { Fragment, type ReactNode } from "react";

interface ProposalOutlookProps {
  assessment?: Assessment;
  pending?: boolean;
  showDetails?: boolean;
  // The household's timezone, so "Checked" reads as the Guide tells times; undefined is the
  // device's own.
  timeZone?: string;
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

const Line = ({ children }: { children: ReactNode }) => (
  <Text as="p" textRole="compact" tone="secondary">
    {children}
  </Text>
);

const ProposalOutlookDiagnostics = ({
  assessment: value,
  timeZone,
}: {
  assessment: Assessment;
  timeZone?: string;
}) => (
  <Stack gap="inline">
    <Grid columnGap="4" columns={["auto", "1fr"]} render="dl" rowGap="1">
      {(
        [
          ["Requested or kept", value.mix.core],
          ["Related suggestions", value.mix.adjacent],
          ["Other suggestions", value.mix.discovery],
          ["Other matches", value.mix.unknown],
        ] as const
      )
        .filter(([, count]) => count > 0)
        .map(([label, count]) => (
          <Fragment key={label}>
            <Text as="dt" textRole="compact" tone="secondary">
              {label}
            </Text>
            <Text as="dd" textRole="compact" tone="secondary">
              {count}
            </Text>
          </Fragment>
        ))}
      <Text as="dt" textRole="compact" tone="secondary">
        Seasons represented
      </Text>
      <Text as="dd" textRole="compact" tone="secondary">
        {value.seasons}
      </Text>
    </Grid>
    <Line>
      This estimate reflects the current title list and schedule settings. Missing titles and breaks do not
      add playing time.
    </Line>
    {value.relaxations.length > 0 && (
      <Line>
        Loomarr loosened {value.relaxations.length} scheduling{" "}
        {value.relaxations.length === 1 ? "rule" : "rules"} to build this preview. Your audience and title
        limits still apply.
      </Line>
    )}
    <Line>
      Checked{" "}
      {new Date(value.observedAt).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone,
      })}
      . Changes to your library or schedule can update this result.
    </Line>
  </Stack>
);

const ProposalOutlookDetails = ({
  assessment: value,
  showDiagnostics = true,
  timeZone,
}: {
  assessment: Assessment;
  showDiagnostics?: boolean;
  timeZone?: string;
}) => (
  <Stack aria-label="What can play now" gap="inline" render="section">
    <Text as="h3" textRole="cardLabel" tone="secondary">
      What can play now
    </Text>
    <Line>
      {value.programs > 0
        ? `${value.programs} playable ${value.programs === 1 ? "episode or movie" : "episodes or movies"} from ${value.scheduledTitles} ${value.scheduledTitles === 1 ? "title" : "titles"}.`
        : value.state === "uncertain"
          ? "We haven't confirmed what's ready to play yet."
          : "No episodes or movies are ready to play yet."}
    </Line>
    {value.missingAcquisitions > 0 && (
      <Line>
        {value.missingAcquisitions}{" "}
        {value.missingAcquisitions === 1 ? "title still needs" : "titles still need"} adding and{" "}
        {value.missingAcquisitions === 1 ? "isn't" : "aren't"} included in the estimate yet.
      </Line>
    )}
    {value.missingLibrary > 0 && (
      <Line>
        {value.missingLibrary} library {value.missingLibrary === 1 ? "title isn't" : "titles aren't"} ready to
        play and {value.missingLibrary === 1 ? "isn't" : "aren't"} included in the estimate.
      </Line>
    )}
    {value.unknownTitles > 0 && (
      <Line>
        Availability hasn't been confirmed for {value.unknownTitles}{" "}
        {value.unknownTitles === 1 ? "title" : "titles"}.{" "}
        {value.unknownTitles === 1 ? "It isn't" : "They aren't"} included in the estimate.
      </Line>
    )}
    {value.windowLimited && <Line>This is a partial preview; more from your library may fit.</Line>}
    {showDiagnostics && (
      <Stack marginTop={4}>
        <Details fitSummary summary="How this estimate works" tone="secondary">
          <Stack marginTop={12}>
            <ProposalOutlookDiagnostics assessment={value} timeZone={timeZone} />
          </Stack>
        </Details>
      </Stack>
    )}
  </Stack>
);

const ProposalOutlook = ({
  assessment: value,
  pending = false,
  showDetails = true,
  timeZone,
}: ProposalOutlookProps) => {
  if (pending)
    return (
      <div role="status">
        <Text as="p" textRole="compact" tone="secondary">
          Checking this lineup against your library…
        </Text>
      </div>
    );
  if (!value)
    return (
      <div role="status">
        <Text as="p" textRole="compact" tone="secondary">
          A schedule preview isn't available yet. You can still review the titles.
        </Text>
      </div>
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
    <Stack aria-label="Channel outlook" render="section">
      <Text as="p" textRole="compact" tone={value.thin ? "warning" : "secondary"}>
        {summary}
      </Text>
      {showDetails && (
        <Stack marginTop={8}>
          <Details summary="Schedule details" tone="secondary">
            <Stack marginTop={8}>
              <ProposalOutlookDetails assessment={value} timeZone={timeZone} />
            </Stack>
          </Details>
        </Stack>
      )}
    </Stack>
  );
};

export { ProposalOutlook, ProposalOutlookDetails, ProposalOutlookDiagnostics };
