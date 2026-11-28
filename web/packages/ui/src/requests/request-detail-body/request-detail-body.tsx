import { pickCount, progressLine, requestAcquisitions } from "@loomarr/core/requests";
import { Badge, type Density, Stack, Surface, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";

import { requestBadgeTone } from "../request-card";
import type { RequestDetailBodyProps } from "./request-detail-body.type";

const titleState = {
  available: { label: "Available", tone: "success" },
  downloading: { label: "Downloading", tone: "warning" },
  requested: { label: "Requested", tone: "warning" },
  unavailable: { label: "Couldn't get", tone: "danger" },
  wanted: { label: "Waiting", tone: "info" },
} as const;

type SectionProps = {
  children: ReactNode;
  density?: Density;
  headingLevel?: "h2";
  title: string;
};

const Section = ({ children, density, headingLevel, title }: SectionProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="inline" role="group">
    <Text as={headingLevel} density={density} textRole="headline">
      {title}
    </Text>
    {children}
  </Surface>
);

// Everything about one request below its header: status, the live stage while generating, why it
// failed, who decided and when, the lineup that was proposed, and the titles still on their way. A
// title that has landed is part of the channel, so only what is outstanding is listed. It neither
// fetches nor scrolls; the frame around it does.
const RequestDetailBody = ({
  actions,
  decisionActions,
  density,
  entry,
  headingLevel,
  progress,
  titleAction,
  titleBadge,
  titles,
  titlesError,
  whenText,
}: RequestDetailBodyProps) => {
  const { journey, status } = entry;
  const proposal = journey.proposal;
  const asked = requestAcquisitions(journey, [...titles]);
  const outstanding = asked.filter(({ title }) => title?.state !== "available");
  const lineup = proposal?.proposal.lineup ?? [];
  const section = { density, headingLevel };

  return (
    <>
      <Badge density={density} tone={requestBadgeTone(status.tone)}>
        {status.line}
      </Badge>

      {journey.milestone === "generating"
        ? (progress ?? (
            <Surface gap="inline" padding="control" level="raised">
              <Text density={density} textRole="body">
                {journey.progress ? progressLine(journey.progress) : "Reading your request…"}
              </Text>
              {journey.progress && pickCount(journey.progress) ? (
                <Text density={density} textRole="caption" tone="secondary">
                  {pickCount(journey.progress)}
                </Text>
              ) : null}
            </Surface>
          ))
        : null}

      {journey.failure ? (
        <Surface gap="inline" padding="control" level="raised">
          <Text density={density} textRole="label">
            {journey.failure.message}
          </Text>
          <Text density={density} textRole="body" tone="secondary">
            {journey.failure.guidance}
          </Text>
        </Surface>
      ) : null}

      {actions}

      {proposal ? (
        <Section {...section} title="Decision">
          {proposal.status === "submitted" ? (
            <Text density={density} textRole="body" tone="secondary">
              Waiting for an admin to approve this request.
            </Text>
          ) : null}
          {decisionActions}
          {proposal.status === "approved" ? (
            <Text density={density} textRole="body">
              {proposal.approvedByName ? `Approved by ${proposal.approvedByName}` : "Approved"}
              {proposal.approvedAt ? ` ${whenText(proposal.approvedAt)}` : ""}
              {proposal.modSummary ? `, with changes: ${proposal.modSummary}` : ""}
            </Text>
          ) : null}
          {proposal.status === "denied" ? (
            <Text density={density} textRole="body" tone="danger">
              {`Not approved${proposal.denyReason ? `: ${proposal.denyReason}` : ". No reason was given."}`}
            </Text>
          ) : null}
          {proposal.note ? (
            <Text density={density} textRole="body">
              {proposal.note}
            </Text>
          ) : null}
        </Section>
      ) : null}

      {proposal ? (
        <Section {...section} title="Proposed lineup">
          {lineup.length === 0 ? (
            <Text density={density} textRole="body" tone="secondary">
              Nothing from your library was proposed.
            </Text>
          ) : (
            <Stack gap="inline" render="ul">
              {lineup.map((item, index) => (
                <Text
                  as="li"
                  density={density}
                  // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal, so the id alone is not a unique key
                  key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                  textRole="body"
                >
                  {item.year ? `${item.name} (${item.year})` : item.name}
                </Text>
              ))}
            </Stack>
          )}
        </Section>
      ) : null}

      {/* An approval that has not produced its titles yet still gets the section, saying so. */}
      {proposal?.status === "approved" && outstanding.length === 0 ? (
        <Section {...section} title="Titles being added">
          <Text density={density} textRole="body" tone="secondary">
            {journey.milestone === "live"
              ? "All titles are on the channel."
              : asked.length > 0
                ? "All titles have arrived."
                : "Titles appear once the channel starts building."}
          </Text>
        </Section>
      ) : null}

      {outstanding.length > 0 ? (
        <Section {...section} title="Titles being added">
          <Stack gap="inline" render="ul">
            {outstanding.map(({ item, title }, index) => (
              <Surface
                alignItems="center"
                flexDirection="row"
                gap="control"
                justifyContent="space-between"
                minHeight={48}
                padding="control"
                // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal
                key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                level="raised"
                render="li"
              >
                <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2}>
                  <Text density={density} textRole="body">
                    {item.year ? `${item.name} (${item.year})` : item.name}
                  </Text>
                  {/* Why it was given up; otherwise the row only says it failed. */}
                  {title?.state === "unavailable" && title.lastError ? (
                    <Text density={density} textRole="caption" tone="secondary">
                      {title.lastError}
                    </Text>
                  ) : null}
                </Surface>
                {title ? (
                  (titleBadge?.(title) ?? (
                    <Badge density={density} tone={titleState[title.state].tone}>
                      {titleState[title.state].label}
                    </Badge>
                  ))
                ) : (
                  <Text density={density} textRole="caption" tone="secondary">
                    Not requested yet
                  </Text>
                )}
                {title ? titleAction?.(title) : null}
              </Surface>
            ))}
          </Stack>
          {titlesError}
        </Section>
      ) : null}
    </>
  );
};

export { RequestDetailBody };
