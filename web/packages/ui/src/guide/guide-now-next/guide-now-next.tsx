import {
  formatGuideTime,
  formatProgrammeTitle,
  type GuideAiringLayout,
  type GuideChannelLayout,
  guideChannelState,
} from "@loomarr/core/guide";
import { Action, ProgressTrack, Surface, semanticRadius, Text } from "@loomarr/design-system";
import { useMemo, useState } from "react";
import { Pressable, ScrollView, View } from "react-native";

import { GuideNumberBlock } from "../guide-number-block";
import type { GuideNowNextProps, GuidePhoneHeaderProps } from "./guide-now-next.type";

// GuideNowNext — the guide's first view on phones and desktops (#2766, #2787; the Direction mock's
// variant C): one row per channel saying what is on, how far through it is, and what comes next.
// No time grid and no colour blocks; the full grid is the other view of the same page.

const GUTTER = 12;
const WIDE_GUTTER = 24;
const ROW_GAP = 6;
// The desktop row's columns, from the mock: a fixed channel column, then On now and Next sharing
// the rest 1.4 : 1.
const CHANNEL_COLUMN = 240;
const COLUMN_GAP = 32;
const NEXT_TIME = 80;
const NEXT_TIME_GAP = 16;
const UNTIL_COLUMN = 176;
const MENU = 40;

type NowKind = "break" | "downloading" | "offair" | "paused" | "show";

type NowNextRow = {
  kind: NowKind;
  /** The next programme (not a break) after what is on now. */
  next?: GuideAiringLayout["source"];
  /** 0–100 through what is on now. */
  percent: number;
  minutesLeft: number;
  title: string;
  /** When what is on now ends. */
  untilMs?: number;
};

// "34 min", "1 hr", "1 hr 5 min": the mock's own wording for time left.
const minutesText = (minutes: number) => {
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest === 0 ? `${hours} hr` : `${hours} hr ${rest} min`;
};

const rowOf = (channel: GuideChannelLayout, nowMs: number, timezone?: string): NowNextRow => {
  const { airings } = channel.source;
  const { health } = guideChannelState(channel.source);
  const now = airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs);
  const after = now?.stopMs ?? nowMs;
  const next = airings.find((a) => a.kind === "program" && a.startMs >= after);
  if (health === "creating") {
    return { kind: "downloading", minutesLeft: 0, next, percent: 0, title: "Getting its shows ready" };
  }
  // Nothing on now (#2787): an admin-paused channel says so, and an off-air one says when it is
  // back if the guide lists it. Neither draws a progress bar.
  if (health === "paused") return { kind: "paused", minutesLeft: 0, percent: 0, title: "Paused" };
  if (!now) {
    const back = airings.find((a) => a.startMs >= nowMs);
    const title = back ? `Off air · back at ${formatGuideTime(back.startMs, timezone)}` : "Off air";
    return { kind: "offair", minutesLeft: 0, next, percent: 0, title };
  }
  const percent = ((nowMs - now.startMs) / (now.stopMs - now.startMs)) * 100;
  const minutesLeft = Math.max(1, Math.ceil((now.stopMs - nowMs) / 60_000));
  if (now.kind === "program") {
    return {
      kind: "show",
      minutesLeft,
      next,
      percent,
      title: formatProgrammeTitle(now),
      untilMs: now.stopMs,
    };
  }
  const title = now.kind === "pending" ? formatProgrammeTitle(now) : "Commercial break";
  return { kind: "break", minutesLeft, next, percent, title, untilMs: now.stopMs };
};

// Favourites first, then the rest; each by channel number.
const ordered = (channels: readonly GuideChannelLayout[], favouriteIds: readonly string[] = []) => {
  const favourite = new Set(favouriteIds);
  const rank = (channel: GuideChannelLayout) => (favourite.has(channel.source.channelId) ? 0 : 1);
  return [...channels].sort((a, b) => rank(a) - rank(b) || a.source.number - b.source.number);
};

type ItemProps = {
  channel: GuideChannelLayout;
  nowMs: number;
  onOpenChannelInfo?: (channelId: string) => void;
  onWatch: (channelId: string) => void;
  timezone?: string;
};

type ItemText = {
  nextLine?: string;
  nextTime?: string;
  row: NowNextRow;
};

const itemText = (channel: GuideChannelLayout, nowMs: number, timezone?: string): ItemText => {
  const row = rowOf(channel, nowMs, timezone);
  const nextTime = row.next ? formatGuideTime(row.next.startMs, timezone) : undefined;
  // A paused channel lists nothing next, and an off-air one only what the guide knows of.
  if (row.kind === "paused" || (row.kind === "offair" && !row.next)) return { row };
  const nextLine =
    row.kind === "downloading"
      ? "When its first show arrives"
      : row.next
        ? formatProgrammeTitle(row.next)
        : "Later tonight";
  return { nextLine, nextTime, row };
};

const NowNextItem = ({ channel, nowMs, onOpenChannelInfo, onWatch, timezone }: ItemProps) => {
  const { channelId, name, number } = channel.source;
  const { nextLine, nextTime, row } = itemText(channel, nowMs, timezone);
  // No label of its own: the row is named by what it draws, so a voice-control user can say what
  // they see (WCAG 2.5.3). A written label added words that are not on screen ("On now", "Next at").
  // A tap tunes; a long-press opens the channel's info (#2787).
  return (
    <Pressable
      accessibilityRole="button"
      onLongPress={onOpenChannelInfo ? () => onOpenChannelInfo(channelId) : undefined}
      onPress={() => onWatch(channelId)}
    >
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
          <GuideNumberBlock decade={channel.source.decade} number={number} />
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
        ) : row.kind === "show" || row.kind === "break" ? (
          <ProgressTrack height={3} percent={row.percent} width="100%" />
        ) : null}
        {nextLine === undefined ? null : (
          <View style={{ flexDirection: "row", gap: 8 }}>
            <Text density="touch" textRole="cardTime" tone="secondary">
              {nextTime ?? "—"}
            </Text>
            <Text density="touch" numberOfLines={1} style={{ flex: 1 }} textRole="cardMeta" tone="secondary">
              {nextLine}
            </Text>
          </View>
        )}
      </Surface>
    </Pressable>
  );
};

// The desktop row: Channel, On now and Next in three columns. The ⋯ shows while the pointer is over
// the row or focus is in it, and is a sibling of the row's button, never inside it.
const WideNowNextItem = ({ channel, nowMs, onOpenChannelInfo, onWatch, timezone }: ItemProps) => {
  const { channelId, name, number } = channel.source;
  const { nextLine, nextTime, row } = itemText(channel, nowMs, timezone);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const progress = row.kind === "show" || row.kind === "break";
  return (
    <Surface
      alignItems="center"
      backgroundColor="surfaceRaised"
      borderColor="borderDecorative"
      borderRadius={semanticRadius.cardCompact}
      borderWidth={1}
      flexDirection="row"
      // Web-only events: focus moving between the row's button and its ⋯ stays "in the row".
      onBlur={(event) => {
        const row = event.currentTarget as unknown as { contains: (node: unknown) => boolean };
        if (!row.contains(event.relatedTarget)) setFocused(false);
      }}
      onFocus={() => setFocused(true)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <Pressable
        accessibilityRole="button"
        onLongPress={onOpenChannelInfo ? () => onOpenChannelInfo(channelId) : undefined}
        onPress={() => onWatch(channelId)}
        style={{
          alignItems: "center",
          flex: 1,
          flexDirection: "row",
          gap: COLUMN_GAP,
          minWidth: 0,
          paddingLeft: 16,
          paddingRight: onOpenChannelInfo ? 8 : WIDE_GUTTER,
          paddingVertical: 16,
        }}
      >
        <View style={{ alignItems: "baseline", flexDirection: "row", gap: 12, width: CHANNEL_COLUMN }}>
          <Text strong style={{ textAlign: "right", width: 28 }} textRole="data" tone="secondary">
            {String(number)}
          </Text>
          <Text style={{ flexShrink: 1, minWidth: 0 }} textRole="label" tone="primary">
            {name}
          </Text>
        </View>
        <View style={{ flex: 1.4, gap: 8, minWidth: 0 }}>
          <Text
            numberOfLines={1}
            strong
            textRole="headline"
            tone={row.kind === "show" ? "primary" : "secondary"}
          >
            {row.title}
          </Text>
          {row.kind === "downloading" ? (
            <Text numberOfLines={1} textRole="body" tone="secondary">
              Commercials play until its shows arrive.
            </Text>
          ) : progress ? (
            <View style={{ alignItems: "center", flexDirection: "row", gap: 16 }}>
              <View style={{ flex: 1, minWidth: 0 }}>
                <ProgressTrack height={3} percent={row.percent} width="100%" />
              </View>
              <Text style={{ width: UNTIL_COLUMN }} textRole="cardMeta" tone="secondary">
                {row.kind === "show" && row.untilMs !== undefined
                  ? `until ${formatGuideTime(row.untilMs, timezone)} · ${minutesText(row.minutesLeft)} left`
                  : `Back in ${minutesText(row.minutesLeft)}`}
              </Text>
            </View>
          ) : null}
        </View>
        <View
          style={{ alignItems: "baseline", flex: 1, flexDirection: "row", gap: NEXT_TIME_GAP, minWidth: 0 }}
        >
          <Text style={{ textAlign: "right", width: NEXT_TIME }} textRole="data" tone="secondary">
            {nextTime ?? ""}
          </Text>
          <Text numberOfLines={1} style={{ flex: 1 }} textRole="reading" tone="primary">
            {nextLine ?? ""}
          </Text>
        </View>
      </Pressable>
      {onOpenChannelInfo ? (
        <View style={{ marginRight: 8, opacity: hovered || focused ? 1 : 0, width: MENU }}>
          <Action icon="more" layout="icon" onPress={() => onOpenChannelInfo(channelId)} variant="ghost">
            {`Channel info for ${name}`}
          </Action>
        </View>
      ) : null}
    </Surface>
  );
};

// The desktop list's column headings, over the same columns as its rows. Hidden from assistive
// tech: each row already reads in that order.
const WideHeadings = ({ menu }: { menu: boolean }) => (
  <View
    aria-hidden
    style={{
      flexDirection: "row",
      gap: COLUMN_GAP,
      paddingLeft: 16,
      paddingRight: menu ? 8 + MENU + 8 : WIDE_GUTTER,
    }}
  >
    <View style={{ width: CHANNEL_COLUMN }}>
      <Text textRole="cardMeta" tone="secondary">
        Channel
      </Text>
    </View>
    <View style={{ flex: 1.4, minWidth: 0 }}>
      <Text textRole="cardMeta" tone="secondary">
        On now
      </Text>
    </View>
    <View style={{ flex: 1, minWidth: 0, paddingLeft: NEXT_TIME + NEXT_TIME_GAP }}>
      <Text textRole="cardMeta" tone="secondary">
        Next
      </Text>
    </View>
  </View>
);

const GuideNowNext = ({
  favouriteIds,
  layout,
  nowMs,
  onOpenChannelInfo,
  onWatch,
  wide,
}: GuideNowNextProps) => {
  const channels = useMemo(() => ordered(layout.channels, favouriteIds), [layout.channels, favouriteIds]);
  const Item = wide ? WideNowNextItem : NowNextItem;
  return (
    <ScrollView
      accessibilityLabel="Channel schedule"
      contentContainerStyle={{
        gap: ROW_GAP,
        paddingHorizontal: wide ? WIDE_GUTTER : GUTTER,
        paddingVertical: 4,
      }}
      style={{ flex: 1, minHeight: 0 }}
    >
      {wide ? <WideHeadings menu={onOpenChannelInfo !== undefined} /> : null}
      {channels.map((channel) => (
        <Item
          channel={channel}
          key={channel.source.channelId}
          nowMs={nowMs}
          onOpenChannelInfo={onOpenChannelInfo}
          onWatch={onWatch}
          timezone={layout.timezone}
        />
      ))}
    </ScrollView>
  );
};

// The guide's heading: its title and a line under it, with the view switch to the right.
const GuidePhoneHeader = ({ children, subtitle, title, wide }: GuidePhoneHeaderProps) => (
  <View
    style={{
      alignItems: "flex-end",
      flexDirection: "row",
      gap: 12,
      justifyContent: "space-between",
      paddingBottom: 8,
      paddingHorizontal: wide ? WIDE_GUTTER : GUTTER,
      paddingTop: wide ? 20 : 16,
    }}
  >
    <View style={{ flexShrink: 1, gap: 2, minWidth: 0 }}>
      <Text as="h2" density={wide ? "pointer" : "touch"} textRole="title">
        {title}
      </Text>
      <Text density={wide ? "pointer" : "touch"} textRole="cardMeta" tone="secondary">
        {subtitle}
      </Text>
    </View>
    {children ? <View style={{ flexShrink: 0, width: 224 }}>{children}</View> : null}
  </View>
);

export { GuideNowNext, GuidePhoneHeader };
