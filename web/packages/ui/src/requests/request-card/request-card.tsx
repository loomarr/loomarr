import { guideWallClock } from "@loomarr/core/guide";
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

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"] as const;

/**
 * "Oct 3": the calendar day a moment falls on in the household's zone, so a request reads the same
 * day as the Guide's times do for a viewer in another zone. The device's zone while undefined.
 */
const calendarDay = (iso: string, timeZone?: string): string => {
  const { day, month } = guideWallClock(new Date(iso).getTime(), timeZone);
  return `${MONTHS[month - 1]} ${day}`;
};

/** "2 days ago", the relative date the approved mock shows; a week or more is the calendar day. */
const requestedOn = (iso: string, nowMs: number = Date.now(), timeZone?: string): string => {
  const at = new Date(iso);
  const age = nowMs - at.getTime();
  const ago = (n: number, unit: string) => `${n} ${unit}${n === 1 ? "" : "s"} ago`;
  if (age < MINUTE) return "Requested just now";
  if (age < HOUR) return `Requested ${ago(Math.floor(age / MINUTE), "minute")}`;
  if (age < DAY) return `Requested ${ago(Math.floor(age / HOUR), "hour")}`;
  if (age < 7 * DAY) return `Requested ${ago(Math.floor(age / DAY), "day")}`;
  return `Requested ${calendarDay(iso, timeZone)}`;
};

// One request: what was asked, where it stands, and the one thing to do next. The whole card is the
// target for opening the request (at least 48 pt tall); its action is a second, separate target.
const RequestCard = ({ action, entry, hint, nowMs, onOpen, timeZone, trailing }: RequestCardProps) => {
  const { journey, status } = entry;
  const detail = hint ?? status.detail;
  const body = (
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="control">
      {/* A request has no artwork of its own, and the shared ArtworkFrame's "No artwork" fallback is a
          card-sized state, so the phone leaves the slot out rather than draw a placeholder. */}
      <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap="inline">
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
          {requestedOn(journey.createdAt, nowMs, timeZone)}
        </Text>
      </Surface>
    </Surface>
  );
  return (
    <Surface gap="control" padding="control" width="100%" level="raised">
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

export { calendarDay, RequestCard, requestBadgeTone, requestedOn };
