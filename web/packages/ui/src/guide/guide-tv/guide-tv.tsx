import {
  formatGuideEpisode,
  formatGuideTime,
  formatGuideTimeRange,
  guideAiringLabel,
} from "@loomarr/core/guide";
import { Action, ArtworkFrame, Surface, Text } from "@loomarr/design-system";
import { ScrollView, View } from "react-native";

import type { GuideFilterOption, GuideSurfaceProps } from "../guide.type";

const channelRailPercent = 31;
const rowHeight = 96;

const TvGuideSurface = ({
  channelWindow,
  filter = "all",
  filters,
  focusRegistry,
  layout,
  onFilterChange,
  onSelectionChange,
  onTune,
  renderArtwork,
  selection,
}: GuideSurfaceProps & { filters: readonly GuideFilterOption[] }) => {
  const selectedChannel = layout.channels.find((channel) => channel.source.channelId === selection.channelId);
  const selectedAiring = selectedChannel?.airings.find(
    (airing) => airing.scheduleBlockId === selection.scheduleBlockId,
  );
  const artwork = selectedAiring ? renderArtwork?.(selectedAiring) : undefined;
  const visibleChannels = channelWindow
    ? layout.channels.slice(channelWindow.start, channelWindow.end)
    : layout.channels;
  const span = layout.toMs - layout.fromMs;
  const ticks = [layout.fromMs, layout.fromMs + span / 2, layout.toMs];
  const onNow = layout.channels.flatMap((channel) => channel.airings).find((airing) => airing.isOnNow);
  const nowMs =
    onNow?.progressRatio === undefined
      ? undefined
      : onNow.source.startMs + onNow.progressRatio * (onNow.source.stopMs - onNow.source.startMs);
  const nowPercent = nowMs === undefined ? undefined : ((nowMs - layout.fromMs) / span) * 100;

  return (
    <Surface
      aria-label="Programme guide"
      borderRadius={0}
      borderWidth={0}
      flex={1}
      level="canvas"
      overflow="hidden"
    >
      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        gap="$control"
        paddingBottom="$control"
        paddingHorizontal={48}
        paddingTop={40}
      >
        <Text density="tv" textRole="display">
          Guide
        </Text>
        {filters.map((option) => {
          const count = option.value === "all" ? layout.channels.length : 0;
          const name = option.value === "favourites" ? "Favorites" : option.label;
          const label = `${option.value === "favourites" ? "★ " : ""}${name} · ${count}`;
          return (
            <Action
              accessibilityLabel={`${name} channels`}
              density="tv"
              disabled={option.disabled}
              hasTVPreferredFocus={filter === option.value && layout.channels.length === 0}
              key={option.value}
              onPress={() => onFilterChange?.(option.value)}
              ref={(handle) => focusRegistry?.register({ filter: option.value, kind: "filter" }, handle)}
              selected={filter === option.value}
              tone="secondary"
            >
              {label}
            </Action>
          );
        })}
        <Text density="tv" flex={1} textAlign="right" textRole="metadata" tone="muted">
          {channelWindow?.positionLabel ?? "▲ Filters"}
        </Text>
      </Surface>

      <Surface
        backgroundColor="$transparent"
        borderRadius={0}
        borderWidth={0}
        flex={1}
        minHeight={0}
        position="relative"
      >
        <View style={{ flexDirection: "row", height: 64, paddingHorizontal: 48 }}>
          <View style={{ justifyContent: "center", width: `${channelRailPercent}%` }}>
            <Text density="tv" textRole="metadata" tone="muted">
              CHANNEL
            </Text>
          </View>
          <View style={{ flex: 1, flexDirection: "row", justifyContent: "space-between" }}>
            {ticks.map((tick) => (
              <Text density="tv" key={tick} textRole="time" tone="muted">
                {formatGuideTime(tick, layout.timezone)}
              </Text>
            ))}
          </View>
        </View>

        <ScrollView style={{ flex: 1 }}>
          <Surface aria-label="Channel schedule" borderRadius={0} gap={2} role="group">
            {visibleChannels.map((channel) => (
              <View
                key={channel.source.channelId}
                style={{ flexDirection: "row", height: rowHeight, minWidth: 0 }}
              >
                <Surface
                  alignItems="center"
                  backgroundColor="$surfaceRaised"
                  borderRadius={0}
                  borderWidth={0}
                  flexDirection="row"
                  gap="$control"
                  paddingLeft={48}
                  width={`${channelRailPercent}%`}
                >
                  <Text density="tv" textRole="channelNumber" tone="muted">
                    {String(channel.source.number).padStart(2, "0")}
                  </Text>
                  <Text density="tv" flex={1} numberOfLines={1} textRole="body">
                    {channel.source.name}
                  </Text>
                </Surface>
                <View style={{ flex: 1, minWidth: 0, position: "relative" }}>
                  {channel.airings.length === 0 ? (
                    <Text density="tv" textRole="body" tone="muted">
                      Nothing scheduled
                    </Text>
                  ) : null}
                  {channel.airings.map((airing) => {
                    const selected =
                      channel.source.channelId === selection.channelId &&
                      airing.scheduleBlockId === selection.scheduleBlockId;
                    const label = guideAiringLabel(airing.source);
                    const next = {
                      anchorMs: airing.source.startMs + (airing.source.stopMs - airing.source.startMs) / 2,
                      channelId: channel.source.channelId,
                      scheduleBlockId: airing.scheduleBlockId,
                    };
                    const target = { kind: "airing" as const, selection: next };
                    return (
                      <Action
                        accessibilityLabel={`${channel.source.name}, ${label}, ${formatGuideTimeRange(
                          airing.source.startMs,
                          airing.source.stopMs,
                          layout.timezone,
                        )}`}
                        density="tv"
                        hasTVPreferredFocus={selected}
                        key={airing.scheduleBlockId}
                        onFocus={() => onSelectionChange(next)}
                        onPress={() => {
                          onSelectionChange(next);
                          onTune?.(next);
                        }}
                        ref={(handle) => focusRegistry?.register(target, handle)}
                        selected={selected}
                        style={{
                          height: rowHeight - 4,
                          left: `${airing.startRatio * 100}%`,
                          minHeight: 0,
                          overflow: "hidden",
                          paddingHorizontal: 20,
                          position: "absolute",
                          top: 2,
                          width: `${airing.widthRatio * 100}%`,
                        }}
                        tone="secondary"
                      >
                        {airing.widthRatio >= 0.08 ? label : ""}
                      </Action>
                    );
                  })}
                </View>
              </View>
            ))}
          </Surface>
        </ScrollView>

        {nowPercent === undefined ? null : (
          <Surface
            backgroundColor="$stateLive"
            borderRadius={0}
            borderWidth={0}
            bottom={0}
            left={`${channelRailPercent + (nowPercent * (100 - channelRailPercent)) / 100}%`}
            position="absolute"
            top={64}
            width={4}
          />
        )}
      </Surface>

      {selectedAiring && selectedChannel ? (
        <Surface
          borderRadius={0}
          flexDirection="row"
          gap="$control"
          height={220}
          paddingHorizontal={48}
          paddingVertical="$control"
        >
          <ArtworkFrame density="tv" state={artwork ? "ready" : "missing"} width={272}>
            {artwork}
          </ArtworkFrame>
          <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
            <Text density="tv" numberOfLines={1} textRole="title">
              {selectedAiring.source.title.trim() || guideAiringLabel(selectedAiring.source)}
            </Text>
            <Text density="tv" numberOfLines={1} textRole="time">
              {[
                selectedAiring.source.series,
                formatGuideEpisode(selectedAiring.source.season, selectedAiring.source.episode),
                selectedAiring.source.year ? String(selectedAiring.source.year) : undefined,
                selectedAiring.source.rating,
              ]
                .filter(Boolean)
                .join(" · ")}
            </Text>
            <Text density="tv" numberOfLines={1} textRole="metadata">
              {[
                selectedAiring.source.genres?.slice(0, 2).join(" / "),
                formatGuideTimeRange(
                  selectedAiring.source.startMs,
                  selectedAiring.source.stopMs,
                  layout.timezone,
                ),
                `CH ${selectedChannel.source.number}`,
              ]
                .filter(Boolean)
                .join(" · ")}
            </Text>
            {selectedAiring.source.description ? (
              <Text density="tv" numberOfLines={2} textRole="body" tone="muted">
                {selectedAiring.source.description}
              </Text>
            ) : null}
          </Surface>
        </Surface>
      ) : null}
    </Surface>
  );
};

export { TvGuideSurface };
