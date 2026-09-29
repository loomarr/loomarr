import {
  formatGuideTime,
  formatGuideTimeRange,
  type GuideAiringLayout,
  type GuideSelection,
  guideAiringLabel,
} from "@loomarr/core/guide";
import { Action, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { type LayoutChangeEvent, Pressable, ScrollView, View } from "react-native";

import { guideFilterChannelIds, guideFilterOptions, guideFilterText } from "../guide-filter";
import type { GuideCompactProps } from "./guide-compact.type";

// GuideCompact — the phone guide (#1659 native mocks 5d/5f, maintainer decision N6): a narrow
// channel-number column, the time ruler with its now badge, programme cells scaled to their
// length, the filter chips with their counts, and the selected programme docked above the page's
// foot with Watch (5f's docked strip). Tapping a cell selects it; Watch tunes its channel.
//
// A row is the 44 pt target the decision asks for (48 tall, the cell drawn 3 px inside it); a
// programme shorter than that stays as narrow as its time, and the dock carries its full title.

const GUTTER = 16;
const NUMBER_COLUMN = 40;
const ROW = 48;
const RULER = 24;
const hourMs = 3_600_000;
// An hour label ("10 PM") needs this much room; the now badge ("8:26 PM") this much either side.
const LABEL_MIN_PX = 48;
const LABEL_STRIDES = [1, 2, 3, 4, 6] as const;
const BADGE_CLEAR_PX = 56;

const selectionOf = (airing: GuideAiringLayout): GuideSelection => ({
  anchorMs: airing.source.startMs + (airing.source.stopMs - airing.source.startMs) / 2,
  channelId: airing.channelId,
  scheduleBlockId: airing.scheduleBlockId,
});

// "Series “Episode”" as the mocks write a programme; a film or a break is its label alone.
const programmeLine = (airing: GuideAiringLayout) =>
  airing.source.series && airing.source.title.trim()
    ? `${airing.source.series} “${airing.source.title.trim()}”`
    : guideAiringLabel(airing.source);

// The sheet's time line (5d): "8:50–9:39 PM · in 24m · TV-G". A programme on now counts down as
// Watching's rows do ("34m left"); one that has ended says nothing more.
const sheetTimeLine = (airing: GuideAiringLayout, nowMs: number, timezone?: string) => {
  const { rating, startMs, stopMs } = airing.source;
  const minutes = (ms: number) => `${Math.max(1, Math.ceil(ms / 60_000))}m`;
  const when =
    nowMs < startMs
      ? `in ${minutes(startMs - nowMs)}`
      : nowMs < stopMs
        ? `${minutes(stopMs - nowMs)} left`
        : undefined;
  return [formatGuideTimeRange(startMs, stopMs, timezone), when, rating].filter(Boolean).join(" · ");
};

const GuideCompact = ({
  density = "touch",
  dock = "strip",
  filter: chosenFilter,
  layout,
  myChannels,
  nowMs,
  onFilterChange,
  onSelect,
  onWatch,
  renderArtwork,
  selection,
}: GuideCompactProps) => {
  // The iPhone sheet drags down to close; tapping a programme brings it back.
  const [dismissedBlockId, setDismissedBlockId] = useState<string>();
  const filters = guideFilterOptions(layout, myChannels);
  // A personal filter that empties (the last favourite unstarred) falls back to All.
  const filter = filters.find((option) => option.value === chosenFilter)?.disabled ? "all" : chosenFilter;
  const keep = guideFilterChannelIds(filter, myChannels);
  const kept = keep ? new Set(keep) : undefined;
  const channels = kept ? layout.channels.filter((c) => kept.has(c.source.channelId)) : layout.channels;

  const span = Math.max(1, layout.toMs - layout.fromMs);
  const nowRatio = nowMs >= layout.fromMs && nowMs < layout.toMs ? (nowMs - layout.fromMs) / span : undefined;
  const ticks: number[] = [];
  for (let t = Math.ceil(layout.fromMs / hourMs) * hourMs; t < layout.toMs; t += hourMs) ticks.push(t);
  // Hours are labelled as the mocks write them ("9 PM"), every second or third one when an hour is
  // too narrow for its label, and not at all where the now badge sits over it.
  const [timelineWidth, setTimelineWidth] = useState(0);
  const hourPx = (timelineWidth * hourMs) / span;
  const stride = hourPx <= 0 ? 1 : (LABEL_STRIDES.find((s) => s * hourPx >= LABEL_MIN_PX) ?? 6);
  const labelled = (t: number, index: number) =>
    index % stride === 0 &&
    (nowRatio === undefined ||
      Math.abs(((t - layout.fromMs) / span - nowRatio) * timelineWidth) >= BADGE_CLEAR_PX);

  const selectedChannel = layout.channels.find((c) => c.source.channelId === selection?.channelId);
  const selectedAiring = selectedChannel?.airings.find(
    (a) => a.scheduleBlockId === selection?.scheduleBlockId,
  );
  const Dock = dock === "strip" ? undefined : dock;

  return (
    <View style={{ flex: 1, minHeight: 0 }}>
      <View
        accessibilityLabel="Guide filters"
        role="toolbar"
        style={{ flexDirection: "row", gap: 8, paddingBottom: 4, paddingHorizontal: GUTTER }}
      >
        {filters.map((option) => {
          const { accessibilityLabel, text } = guideFilterText(option);
          const selected = option.value === filter;
          return (
            <Pressable
              accessibilityLabel={accessibilityLabel}
              accessibilityRole="button"
              accessibilityState={{ disabled: option.disabled, selected }}
              aria-pressed={selected}
              disabled={option.disabled}
              key={option.value}
              onPress={() => onFilterChange(option.value)}
              style={{ justifyContent: "center", minHeight: 44, opacity: option.disabled ? 0.55 : 1 }}
            >
              <Surface
                alignItems="center"
                backgroundColor={selected ? "$contentPrimary" : "$transparent"}
                borderColor="$borderDecorative"
                borderRadius={8}
                borderWidth={selected ? 0 : 1}
                height={32}
                justifyContent="center"
                paddingHorizontal={12}
              >
                <Text density={density} textRole="cardLabel" tone={selected ? "inverse" : "secondary"}>
                  {text}
                </Text>
              </Surface>
            </Pressable>
          );
        })}
      </View>

      <View style={{ flexDirection: "row", height: RULER, paddingHorizontal: GUTTER }}>
        <View style={{ width: NUMBER_COLUMN }} />
        <View
          onLayout={(e: LayoutChangeEvent) => setTimelineWidth(e.nativeEvent.layout.width)}
          style={{ flex: 1, position: "relative" }}
        >
          {ticks.map((t, index) =>
            labelled(t, index) ? (
              <Text
                density={density}
                key={t}
                style={{ left: `${((t - layout.fromMs) / span) * 100}%`, position: "absolute", top: 3 }}
                textRole="guideMeta"
              >
                {formatGuideTime(t, layout.timezone).replace(":00", "")}
              </Text>
            ) : null,
          )}
          {nowRatio === undefined ? null : (
            <Surface
              backgroundColor="$guideOnAir"
              borderRadius={4}
              borderWidth={0}
              left={`${nowRatio * 100}%`}
              paddingHorizontal={5}
              position="absolute"
              style={{ transform: [{ translateX: "-50%" }] }}
              top={0}
            >
              <Text density={density} textRole="guideIdent" tone="inverse">
                {formatGuideTime(nowMs, layout.timezone)}
              </Text>
            </Surface>
          )}
        </View>
      </View>

      <ScrollView
        accessibilityLabel="Channel schedule"
        contentContainerStyle={{ paddingHorizontal: GUTTER }}
        style={{ flex: 1, minHeight: 0 }}
      >
        <View style={{ position: "relative" }}>
          {channels.map((channel) => {
            const rowSelected = channel.source.channelId === selection?.channelId;
            return (
              <View key={channel.source.channelId} style={{ flexDirection: "row", height: ROW }}>
                <View style={{ justifyContent: "center", width: NUMBER_COLUMN }}>
                  <Text density={density} textRole="time" tone={rowSelected ? "signal" : "muted"}>
                    {String(channel.source.number)}
                  </Text>
                </View>
                <View style={{ flex: 1, minWidth: 0, position: "relative" }}>
                  {channel.airings.map((airing) => {
                    const selected = rowSelected && airing.scheduleBlockId === selection?.scheduleBlockId;
                    const label = guideAiringLabel(airing.source);
                    return (
                      <Pressable
                        accessibilityLabel={`${channel.source.number} ${channel.source.name}, ${label}, ${formatGuideTimeRange(
                          airing.source.startMs,
                          airing.source.stopMs,
                          layout.timezone,
                        )}`}
                        accessibilityRole="button"
                        accessibilityState={{ selected }}
                        aria-pressed={selected}
                        key={airing.scheduleBlockId}
                        onPress={() => {
                          setDismissedBlockId(undefined);
                          onSelect(selectionOf(airing));
                        }}
                        style={{
                          bottom: 0,
                          left: `${airing.startRatio * 100}%`,
                          paddingRight: 3,
                          paddingVertical: 3,
                          position: "absolute",
                          top: 0,
                          width: `${airing.widthRatio * 100}%`,
                        }}
                      >
                        <Surface
                          backgroundColor={
                            selected
                              ? "$surfaceFocus"
                              : airing.isOnNow
                                ? "$surfaceElevated"
                                : "$surfaceRaised"
                          }
                          borderColor={selected ? "$actionPrimary" : "$transparent"}
                          borderRadius={4}
                          borderWidth={2}
                          flex={1}
                          justifyContent="center"
                          paddingHorizontal={6}
                        >
                          <Text
                            density={density}
                            numberOfLines={1}
                            textAlign="left"
                            textRole="guideLabel"
                            tone={selected || airing.isOnNow ? "primary" : "secondary"}
                          >
                            {label}
                          </Text>
                        </Surface>
                      </Pressable>
                    );
                  })}
                </View>
              </View>
            );
          })}
          {nowRatio === undefined ? null : (
            <View
              pointerEvents="none"
              style={{ bottom: 0, left: NUMBER_COLUMN, position: "absolute", right: 0, top: 0 }}
            >
              <Surface
                backgroundColor="$guideOnAir"
                borderWidth={0}
                bottom={0}
                left={`${nowRatio * 100}%`}
                marginLeft={-1}
                position="absolute"
                top={0}
                width={2}
              />
            </View>
          )}
        </View>
      </ScrollView>

      {selectedChannel && selectedAiring && Dock ? (
        dismissedBlockId === selectedAiring.scheduleBlockId ? null : (
          <Dock
            accessibilityLabel="Selected programme"
            onDismiss={() => setDismissedBlockId(selectedAiring.scheduleBlockId)}
          >
            <View style={{ flexDirection: "row", gap: 12 }}>
              {renderArtwork ? (
                <Surface
                  backgroundColor="$surfaceElevated"
                  borderRadius={6}
                  borderWidth={0}
                  height={63}
                  overflow="hidden"
                  width={112}
                >
                  {renderArtwork(selectedAiring)}
                </Surface>
              ) : null}
              <View style={{ flex: 1, gap: 2, minWidth: 0 }}>
                <Text density={density} numberOfLines={1} textRole="cardMeta">
                  <Text density={density} textRole="cardTime" tone="primary">
                    {String(selectedChannel.source.number)}
                  </Text>
                  {` · ${selectedChannel.source.name}`}
                </Text>
                <Text density={density} numberOfLines={2} textRole="cardTitle">
                  {programmeLine(selectedAiring)}
                </Text>
                {/* Two lines: a range across noon or midnight ("11:30 PM–12:15 AM") would cut off the rating. */}
                <Text density={density} numberOfLines={2} textRole="guideMeta">
                  {sheetTimeLine(selectedAiring, nowMs, layout.timezone)}
                </Text>
              </View>
            </View>
            <Action
              accessibilityLabel={`Watch ${selectedChannel.source.number} ${selectedChannel.source.name} now`}
              density={density}
              onPress={() => onWatch(selectedChannel.source.channelId)}
              tone="primary"
            >
              {`Watch ${selectedChannel.source.number} now`}
            </Action>
          </Dock>
        )
      ) : selectedChannel && selectedAiring ? (
        <Surface
          alignItems="center"
          backgroundColor="$surfaceRaised"
          borderBottomLeftRadius={0}
          borderBottomRightRadius={0}
          borderRadius={12}
          borderWidth={0}
          flexDirection="row"
          gap={12}
          padding={16}
          role="region"
          aria-label="Selected programme"
        >
          {renderArtwork ? (
            <Surface
              backgroundColor="$surfaceElevated"
              borderRadius={4}
              borderWidth={0}
              height={54}
              overflow="hidden"
              width={96}
            >
              {renderArtwork(selectedAiring)}
            </Surface>
          ) : null}
          <View style={{ flex: 1, gap: 2, minWidth: 0 }}>
            <Text density={density} numberOfLines={1} textRole="cardMeta">
              <Text density={density} textRole="cardTime" tone="primary">
                {String(selectedChannel.source.number)}
              </Text>
              {` · ${formatGuideTimeRange(selectedAiring.source.startMs, selectedAiring.source.stopMs, layout.timezone)}`}
            </Text>
            <Text density={density} numberOfLines={1} textRole="cardTitle">
              {programmeLine(selectedAiring)}
            </Text>
          </View>
          <Action
            accessibilityLabel={`Watch ${selectedChannel.source.number} ${selectedChannel.source.name}`}
            density={density}
            onPress={() => onWatch(selectedChannel.source.channelId)}
            tone="primary"
          >
            Watch
          </Action>
        </Surface>
      ) : null}
    </View>
  );
};

export { GuideCompact };
