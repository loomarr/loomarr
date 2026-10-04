import { pickCount, progressLine, requestAcquisitions, requestFixLabel } from "@loomarr/core/requests";
import { Action, Badge, ScrollFrame, Surface, Text } from "@loomarr/design-system";

import { StatePanel } from "../../state-panel";
import { requestBadgeTone, requestedOn } from "../request-card";
import { RequestsBack } from "../requests-back";
import type { RequestDetailProps } from "./request-detail.type";

const titleState = {
  available: { label: "Available", tone: "success" },
  downloading: { label: "Downloading", tone: "warning" },
  requested: { label: "Requested", tone: "warning" },
  unavailable: { label: "Couldn't get", tone: "danger" },
  wanted: { label: "Waiting", tone: "info" },
} as const;

const decidedOn = (iso?: string) =>
  iso ? new Date(iso).toLocaleDateString("en-US", { day: "numeric", month: "short" }) : undefined;

const Section = ({ children, title }: { children: React.ReactNode; title: string }) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline" role="group">
    <Text density="touch" textRole="headline">
      {title}
    </Text>
    {children}
  </Surface>
);

// One request end to end, in Web's order: status, the live stage while generating, why it failed,
// who decided and when, the lineup that was proposed, and the titles still on their way. A title that
// has landed is part of the channel, so only what is outstanding is listed.
const RequestDetail = ({ entry, onBack, onFix, onOpenChannel, titles }: RequestDetailProps) => {
  const back = <RequestsBack onPress={onBack} />;
  if (!entry)
    return (
      <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap="$control">
        {back}
        <StatePanel
          action={{ label: "Back to Requests", onPress: onBack }}
          density="touch"
          description="It may have been removed. Your other requests are still in the list."
          kind="empty"
          title="That request isn't here"
        />
      </Surface>
    );

  const { journey, status } = entry;
  const proposal = journey.proposal;
  const fix = requestFixLabel(journey);
  const asked = requestAcquisitions(journey, [...titles]);
  const outstanding = asked.filter(({ title }) => title?.state !== "available");
  const lineup = proposal?.proposal.lineup ?? [];
  const approvedOn = decidedOn(proposal?.approvedAt);

  return (
    <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap="$control">
      {back}
      <ScrollFrame density="touch">
        <Text density="touch" textRole="display">
          {journey.intent.description}
        </Text>
        <Text density="touch" textRole="caption" tone="secondary">
          {requestedOn(journey.createdAt)}
        </Text>
        <Badge density="touch" tone={requestBadgeTone(status.tone)}>
          {status.line}
        </Badge>

        {journey.milestone === "generating" ? (
          <Surface gap="$inline" level="raised" padding="$control">
            <Text density="touch" textRole="body">
              {journey.progress ? progressLine(journey.progress) : "Reading your request…"}
            </Text>
            {journey.progress && pickCount(journey.progress) ? (
              <Text density="touch" textRole="caption" tone="secondary">
                {pickCount(journey.progress)}
              </Text>
            ) : null}
          </Surface>
        ) : null}

        {journey.failure ? (
          <Surface gap="$inline" level="raised" padding="$control">
            <Text density="touch" textRole="label">
              {journey.failure.message}
            </Text>
            <Text density="touch" textRole="body" tone="secondary">
              {journey.failure.guidance}
            </Text>
          </Surface>
        ) : null}

        {journey.channel ? (
          <Action
            accessibilityRole="button"
            density="touch"
            onPress={() => onOpenChannel(journey.channel?.id ?? "")}
            tone="primary"
          >
            Open channel
          </Action>
        ) : null}
        {fix ? (
          <Action accessibilityRole="button" density="touch" onPress={() => onFix(entry)} tone="secondary">
            {fix}
          </Action>
        ) : null}

        {proposal ? (
          <Section title="Decision">
            {proposal.status === "submitted" ? (
              <Text density="touch" textRole="body" tone="secondary">
                Waiting for an admin to approve this request.
              </Text>
            ) : null}
            {proposal.status === "approved" ? (
              <Text density="touch" textRole="body">
                {proposal.approvedByName ? `Approved by ${proposal.approvedByName}` : "Approved"}
                {approvedOn ? ` on ${approvedOn}` : ""}
                {proposal.modSummary ? `, with changes: ${proposal.modSummary}` : ""}
              </Text>
            ) : null}
            {proposal.status === "denied" ? (
              <Text density="touch" textRole="body" tone="danger">
                {`Not approved${proposal.denyReason ? `: ${proposal.denyReason}` : ". No reason was given."}`}
              </Text>
            ) : null}
            {proposal.note ? (
              <Text density="touch" textRole="body">
                {proposal.note}
              </Text>
            ) : null}
          </Section>
        ) : null}

        {proposal ? (
          <Section title="Proposed lineup">
            {lineup.length === 0 ? (
              <Text density="touch" textRole="body" tone="secondary">
                Nothing from your library was proposed.
              </Text>
            ) : (
              lineup.map((item, index) => (
                <Text
                  density="touch"
                  // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal, so the id alone is not a unique key
                  key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                  textRole="body"
                >
                  {item.year ? `${item.name} (${item.year})` : item.name}
                </Text>
              ))
            )}
          </Section>
        ) : null}

        {proposal?.status === "approved" && outstanding.length === 0 ? (
          <Section title="Titles being added">
            <Text density="touch" textRole="body" tone="secondary">
              {journey.milestone === "live"
                ? "All titles are on the channel."
                : asked.length > 0
                  ? "All titles have arrived."
                  : "Titles appear once the channel starts building."}
            </Text>
          </Section>
        ) : null}

        {outstanding.length > 0 ? (
          <Section title="Titles being added">
            {outstanding.map(({ item, title }, index) => (
              <Surface
                alignItems="center"
                flexDirection="row"
                gap="$control"
                justifyContent="space-between"
                // biome-ignore lint/suspicious/noArrayIndexKey: a title can repeat within a proposal
                key={`${index}:${item.mediaType}:${item.tmdbId ?? item.name}`}
                level="raised"
                minHeight={48}
                padding="$control"
              >
                <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2}>
                  <Text density="touch" textRole="body">
                    {item.year ? `${item.name} (${item.year})` : item.name}
                  </Text>
                  {title?.state === "unavailable" && title.lastError ? (
                    <Text density="touch" textRole="caption" tone="secondary">
                      {title.lastError}
                    </Text>
                  ) : null}
                </Surface>
                {title ? (
                  <Badge density="touch" tone={titleState[title.state].tone}>
                    {titleState[title.state].label}
                  </Badge>
                ) : (
                  <Text density="touch" textRole="caption" tone="secondary">
                    Not requested yet
                  </Text>
                )}
              </Surface>
            ))}
          </Section>
        ) : null}
      </ScrollFrame>
    </Surface>
  );
};

export { RequestDetail };
