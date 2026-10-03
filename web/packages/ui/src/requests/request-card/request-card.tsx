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

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** "2 days ago", the relative date the approved mock shows; a week or more is the calendar day. */
const requestedOn = (iso: string, nowMs: number = Date.now()): string => {
  const at = new Date(iso);
  const age = nowMs - at.getTime();
  const ago = (n: number, unit: string) => `${n} ${unit}${n === 1 ? "" : "s"} ago`;
  if (age < MINUTE) return "Requested just now";
  if (age < HOUR) return `Requested ${ago(Math.floor(age / MINUTE), "minute")}`;
  if (age < DAY) return `Requested ${ago(Math.floor(age / HOUR), "hour")}`;
  if (age < 7 * DAY) return `Requested ${ago(Math.floor(age / DAY), "day")}`;
  return `Requested ${at.toLocaleDateString("en-US", { day: "numeric", month: "short" })}`;
};

// One request: what was asked, where it stands, and the one thing to do next. The whole card is the
// target for opening the request (at least 48 pt tall); its action is a second, separate target.
const RequestCard = ({ action, entry, hint, nowMs, onOpen, trailing }: RequestCardProps) => {
  const { journey, status } = entry;
  const detail = hint ?? status.detail;
  const body = (
    <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$control">
      {/* Titles carry no artwork yet: two offset placeholder tiles, as the approved mock draws them. */}
      <Surface aria-hidden backgroundColor="$transparent" borderWidth={0} width={36}>
        <Surface backgroundColor="$surfaceElevated" height={40} width={28} />
        <Surface backgroundColor="$surfaceElevated" height={40} marginLeft={8} marginTop={-24} width={28} />
      </Surface>
      <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap="$inline">
        <Badge density="touch" tone={requestBadgeTone(status.tone)}>
          {status.line}
        </Badge>
        {/* Two lines, then an ellipsis, so a long brief never pushes the action off the card. */}
        <Text density="touch" numberOfLines={2} textRole="title">
          {journey.intent.description}
        </Text>
        {detail ? (
          <Text density="touch" textRole="body" tone="secondary">
            {detail}
          </Text>
        ) : null}
        <Text density="touch" textRole="caption" tone="secondary">
          {requestedOn(journey.createdAt, nowMs)}
        </Text>
      </Surface>
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
