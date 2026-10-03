import type { RequestStatus } from "@loomarr/core/requests";
import { Action, Badge, type BadgeTone, Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { RequestCardProps } from "./request-card.type";

// Web's four status tones, in the phone's badge vocabulary.
const badgeTones: Record<RequestStatus["tone"], BadgeTone> = {
  caution: "warning",
  lock: "success",
  onair: "danger",
  suggest: "info",
};

const requestBadgeTone = (tone: RequestStatus["tone"]): BadgeTone => badgeTones[tone];

/** "Requested Oct 1": the local calendar day, as the person would say it. */
const requestedOn = (iso: string): string =>
  `Requested ${new Date(iso).toLocaleDateString("en-US", { day: "numeric", month: "short" })}`;

// One request: what was asked, where it stands, and the one thing to do next. The whole card is the
// target for opening the request (at least 48 pt tall); its action is a second, separate target.
const RequestCard = ({ action, entry, hint, onOpen, trailing }: RequestCardProps) => {
  const { journey, status } = entry;
  const detail = hint ?? status.detail;
  const body = (
    <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
      <Text density="touch" textRole="title">
        {journey.intent.description}
      </Text>
      <Badge density="touch" tone={requestBadgeTone(status.tone)}>
        {status.line}
      </Badge>
      <Text density="touch" textRole="caption" tone="secondary">
        {requestedOn(journey.createdAt)}
      </Text>
      {detail ? (
        <Text density="touch" textRole="body" tone="secondary">
          {detail}
        </Text>
      ) : null}
    </Surface>
  );
  return (
    <Surface gap="$control" level="raised" padding="$control" width="100%">
      {onOpen ? (
        <Pressable
          accessibilityHint="Opens this request"
          accessibilityLabel={`${journey.intent.description}, ${status.line}`}
          accessibilityRole="button"
          onPress={onOpen}
          style={{ minHeight: 48 }}
        >
          {body}
        </Pressable>
      ) : (
        body
      )}
      {trailing}
      {action ? (
        <Action accessibilityRole="button" density="touch" onPress={action.onPress} tone="secondary">
          {action.label}
        </Action>
      ) : null}
    </Surface>
  );
};

export { RequestCard, requestBadgeTone, requestedOn };
