import {
  defectBandLabel,
  defectFindingLine,
  defectFindingsToShow,
  defectJudgementLine,
  defectScoreCells,
  defectVerdictSummary,
} from "@loomarr/core/filler-defects";
import { pluralize } from "@loomarr/core/format";
import { Action, AdaptiveSplit, Badge, Surface, Text } from "@loomarr/design-system";

import { StatePanel } from "../../state-panel";
import type { DefectReviewProps } from "./defect-review.type";

// Each column starts at the width of its widest word ("Tape", "Precision", "3 of 4") and shares
// what is left, so all five fit a phone's card without breaking one.
const columns = [
  ["band", "Era", 32],
  ["labelled", "Labelled", 52],
  ["precision", "Precision", 58],
  ["caught", "Caught", 46],
  ["judge", "Judge", 40],
] as const;

// Caption headers, not monospace metadata, for the same room.
const Cell = ({ children, header, width }: { children: string; header?: boolean; width: number }) => (
  <Surface
    backgroundColor="$transparent"
    borderWidth={0}
    flexBasis={width}
    flexGrow={1}
    flexShrink={0}
    role={header ? "columnheader" : "cell"}
  >
    <Text textRole={header ? "caption" : "body"} tone={header ? "secondary" : "primary"}>
      {children}
    </Text>
  </Surface>
);

// The floor against the maintainer's labels, one row per era band that has any (#2034 step 4c).
const Scores = ({ report }: Pick<DefectReviewProps, "report">) =>
  report.scores.length === 0 ? (
    <Text textRole="body" tone="secondary">
      No clips labelled yet. Scores appear here once you label a few below.
    </Text>
  ) : (
    <Surface
      aria-label="Scores against your labels"
      backgroundColor="$transparent"
      borderWidth={0}
      role="table"
    >
      <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline" role="row">
        {columns.map(([key, title, width]) => (
          <Cell header key={key} width={width}>
            {title}
          </Cell>
        ))}
      </Surface>
      {report.scores.map((score) => {
        const cells = defectScoreCells(score);
        return (
          <Surface
            backgroundColor="$transparent"
            borderWidth={0}
            flexDirection="row"
            gap="$inline"
            key={score.band}
            role="row"
          >
            {columns.map(([key, , width]) => (
              <Cell key={key} width={width}>
                {cells[key]}
              </Cell>
            ))}
          </Surface>
        );
      })}
    </Surface>
  );

// The household defect report's labelling queue (#2034 step 4c): the floor's standing and scores,
// then one unlabelled clip at a time with what the floor found, labelled pass or fail against a
// good copy of its era. Labelling or skipping moves to the next.
const DefectReview = ({ busy, error, onLabel, onSkip, player, report }: DefectReviewProps) => {
  const row = report.rows[0];
  return (
    <Surface backgroundColor="$transparent" borderWidth={0} gap="$section">
      <Surface gap="$control" level="raised" padding="$control">
        <Text textRole="label">{defectVerdictSummary(report)}</Text>
        <Scores report={report} />
      </Surface>

      {row ? (
        <Surface gap="$control" level="raised" padding="$control">
          <AdaptiveSplit
            gap="$control"
            primary={player}
            secondary={
              <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
                <Text textRole="title">{row.name}</Text>
                <Text textRole="caption" tone="secondary">
                  {`${row.era || "Era unknown"} · ${defectBandLabel(row.band)} era`}
                </Text>
                <Badge
                  tone={row.verdict === "fail" ? "danger" : row.verdict === "pass" ? "success" : "caution"}
                >
                  {row.verdict === "fail"
                    ? "Would retire"
                    : row.verdict === "pass"
                      ? "Passes"
                      : "Not settled"}
                </Badge>
                {defectFindingsToShow(row.findings).map((finding) => (
                  <Text key={finding.check} textRole="body">
                    {defectFindingLine(finding)}
                  </Text>
                ))}
                {row.judgement ? (
                  <Text textRole="body" tone="secondary">
                    {defectJudgementLine(row.judgement)}
                  </Text>
                ) : null}
                {row.label && !row.label.current ? (
                  <Text textRole="caption" tone="secondary">
                    {`You labelled it ${row.label.label} for ${row.label.era}; its era has changed since.`}
                  </Text>
                ) : null}
              </Surface>
            }
          />
          <Text textRole="body">Would a viewer call this damaged, against a good copy of its era?</Text>
          <Surface
            alignItems="center"
            backgroundColor="$transparent"
            borderWidth={0}
            flexDirection="row"
            flexWrap="wrap"
            gap="$inline"
          >
            <Action
              accessibilityRole="button"
              aria-disabled={busy}
              onPress={() => {
                if (!busy) onLabel(row.clipId, "pass");
              }}
              tone="primary"
            >
              Pass
            </Action>
            <Action
              accessibilityRole="button"
              aria-disabled={busy}
              onPress={() => {
                if (!busy) onLabel(row.clipId, "fail");
              }}
              tone="danger"
            >
              Fail
            </Action>
            <Action accessibilityRole="button" onPress={onSkip} variant="ghost">
              Skip
            </Action>
            <Text textRole="caption" tone="secondary">
              {`${pluralize(report.total, "clip")} left to label`}
            </Text>
          </Surface>
          {error ? (
            <Surface backgroundColor="$transparent" borderWidth={0} role="alert">
              <Text textRole="body" tone="danger">
                {`That label wasn't saved. ${error}`}
              </Text>
            </Surface>
          ) : null}
        </Surface>
      ) : (
        <StatePanel
          description="New clips join the queue once the hourly defect check has measured them."
          kind="empty"
          title="Every measured clip is labelled"
        />
      )}
    </Surface>
  );
};

export { DefectReview };
