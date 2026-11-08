import {
  formatGuideTime,
  type GuideAiringLayout,
  type GuideChannelLayout,
  guideAiringLabel,
} from "@loomarr/core/guide";
import { ProgressTrack, Surface, semanticRadius, Text } from "@loomarr/design-system";
import { Pressable, ScrollView, View } from "react-native";

import type { GuideNowNextProps, GuidePhoneHeaderProps } from "./guide-now-next.type";

// GuideNowNext — the phone guide (#2766, the Direction mock's variant C): one row per channel
// saying what is on, how far through it is, and what comes next. No time grid and no colour
// blocks; the full grid is the other view of the same page.

const GUTTER = 12;
const ROW_GAP = 6;

type NowKind = "break" | "downloading" | "idle" | "show";

type NowNextRow = {
  kind: NowKind;
  /** The next programme (not a break) after what is on now. */
  next?: GuideAiringLayout["source"];
  /** 0–100 through what is on now. */
  percent: number;
  minutesLeft: number;
  title: string;
};

// "34 min", "1 hr", "1 hr 5 min": the mock's own wording for time left.
const minutesText = (minutes: number) => {
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest === 0 ? `${hours} hr` : `${hours} hr ${rest} min`;
};

const rowOf = (channel: GuideChannelLayout, nowMs: number): NowNextRow => {
  const { airings, status } = channel.source;
  const now = airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs);
  const after = now?.stopMs ?? nowMs;
  const next = airings.find((a) => a.kind === "program" && a.startMs >= after);
  if (status === "building") {
    return { kind: "downloading", minutesLeft: 0, next, percent: 0, title: "Getting its shows ready" };
  }
  if (!now) {
    // The mock has no off-air row (a paused channel, or nothing scheduled in the window).
    return { kind: "idle", minutesLeft: 0, next, percent: 0, title: "Nothing on right now" };
  }
  const percent = ((nowMs - now.startMs) / (now.stopMs - now.startMs)) * 100;
  const minutesLeft = Math.max(1, Math.ceil((now.stopMs - nowMs) / 60_000));
  if (now.kind === "program") {
    return { kind: "show", minutesLeft, next, percent, title: guideAiringLabel(now) };
  }
  const title = now.kind === "pending" ? guideAiringLabel(now) : "Commercial break";
  return { kind: "break", minutesLeft, next, percent, title };
};

const NowNextItem = ({
  channel,
  nowMs,
  onWatch,
  timezone,
}: {
  channel: GuideChannelLayout;
  nowMs: number;
  onWatch: (channelId: string) => void;
  timezone?: string;
}) => {
  const { channelId, name, number } = channel.source;
  const row = rowOf(channel, nowMs);
  const nextTime = row.next ? formatGuideTime(row.next.startMs, timezone) : undefined;
  const nextLine =
    row.kind === "downloading"
      ? "When its first show arrives"
      : row.next
        ? guideAiringLabel(row.next)
        : "Later tonight";
  // The row's name is its visible text, in the order it is drawn, so a voice-control user can say
  // what they see (WCAG 2.5.3). Words that are not on screen ("On now", "Next at") broke that.
  const spoken = [
    `${number} ${name}`,
    row.kind === "show" ? `${minutesText(row.minutesLeft)} left` : undefined,
    row.title,
    row.kind === "downloading" ? "Commercials play until its shows arrive" : undefined,
    nextTime ? `${nextTime}: ${nextLine}` : nextLine,
  ]
    .filter(Boolean)
    .join(". ");
  return (
    <Pressable accessibilityLabel={spoken} accessibilityRole="button" onPress={() => onWatch(channelId)}>
      <Surface
        backgroundColor="surfaceRaised"
        borderColor="borderDecorative"
        borderRadius={semanticRadius.cardCompact}
        borderWidth={1}
        gap={4}
        paddingHorizontal={12}
        paddingVertical={10}
      >
        <View style={{ alignItems: "baseline", flexDirection: "row", gap: 6 }}>
          <Text density="touch" textRole="cardChannelNumber" tone="secondary">
            {String(number)}
          </Text>
          <Text density="touch" numberOfLines={1} style={{ flex: 1 }} textRole="cardLabel" tone="secondary">
            {name}
          </Text>
          {row.kind === "show" ? (
            <Text density="touch" textRole="cardLabel" tone="secondary">
              {`${minutesText(row.minutesLeft)} left`}
            </Text>
          ) : null}
        </View>
        <Text
          density="touch"
          numberOfLines={2}
          textRole="label"
          tone={row.kind === "show" ? "primary" : "secondary"}
        >
          {row.title}
        </Text>
        {row.kind === "downloading" ? (
          <Text density="touch" textRole="cardMeta" tone="secondary">
            Commercials play until its shows arrive.
          </Text>
        ) : row.kind === "idle" ? null : (
          <ProgressTrack height={3} percent={row.percent} width="100%" />
        )}
        <View style={{ flexDirection: "row", gap: 8 }}>
          <Text density="touch" textRole="cardTime" tone="secondary">
            {nextTime ?? "—"}
          </Text>
          <Text density="touch" numberOfLines={1} style={{ flex: 1 }} textRole="cardMeta" tone="secondary">
            {nextLine}
          </Text>
        </View>
      </Surface>
    </Pressable>
  );
};

const GuideNowNext = ({ layout, nowMs, onWatch }: GuideNowNextProps) => (
  <ScrollView
    accessibilityLabel="Channel schedule"
    contentContainerStyle={{ gap: ROW_GAP, paddingHorizontal: GUTTER, paddingVertical: 4 }}
    style={{ flex: 1, minHeight: 0 }}
  >
    {layout.channels.map((channel) => (
      <NowNextItem
        channel={channel}
        key={channel.source.channelId}
        nowMs={nowMs}
        onWatch={onWatch}
        timezone={layout.timezone}
      />
    ))}
  </ScrollView>
);

// The phone guide's heading: its title and a line under it, with the view switch to the right.
const GuidePhoneHeader = ({ children, subtitle, title }: GuidePhoneHeaderProps) => (
  <View
    style={{
      alignItems: "flex-end",
      flexDirection: "row",
      gap: 12,
      justifyContent: "space-between",
      paddingBottom: 8,
      paddingHorizontal: GUTTER,
      paddingTop: 16,
    }}
  >
    <View style={{ flexShrink: 1, gap: 2, minWidth: 0 }}>
      <Text as="h2" density="touch" textRole="title">
        {title}
      </Text>
      <Text density="touch" textRole="cardMeta" tone="secondary">
        {subtitle}
      </Text>
    </View>
    {children ? <View style={{ flexShrink: 0, width: 224 }}>{children}</View> : null}
  </View>
);

export { GuideNowNext, GuidePhoneHeader };
