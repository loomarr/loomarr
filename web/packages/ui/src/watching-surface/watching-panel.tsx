import { Action, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { Pressable, View } from "react-native";

import type { WatchingPanelProps } from "./watching-panel.type";
import { behindLabel } from "./watching-surface-state";

// WatchingPanel — what sits UNDER the picture on a portrait phone (#1659 native mock 5e, maintainer
// decision N5): the controls are not drawn on the picture there, so this panel carries them. The
// channel line, what's on with its progress, the four controls (Previous, Channel −, Pause,
// Channel +), what's next, then the viewer's favourites to tune from. Landscape and TV still overlay.

// A series episode reads as the mock writes it: Series “Episode”. A film is its title alone. The
// guide's label already leads with the series ("Series · Episode"), so the episode is what follows.
const programmeParts = ({ seriesTitle, title }: { seriesTitle?: string; title: string }) => {
  const prefix = `${seriesTitle} · `;
  return seriesTitle && title.startsWith(prefix)
    ? { heading: seriesTitle, episode: title.slice(prefix.length) }
    : { heading: title, episode: undefined };
};
const programmeLine = (programme: { seriesTitle?: string; title: string }) => {
  const { heading, episode } = programmeParts(programme);
  return episode ? `${heading} “${episode}”` : heading;
};

const WatchingPanel = ({
  canPrevious,
  canSurf,
  channel,
  clockLabel,
  density,
  favourites,
  live,
  onChannelDown,
  onChannelUp,
  onGoLive,
  onPause,
  onPlay,
  onPrevious,
  onTune,
  schedule,
}: WatchingPanelProps) => {
  const now = schedule?.now;
  const parts = now ? programmeParts(now) : undefined;
  // The mock's line is episode · year · time: one fact (the year, first when known), not all of them.
  const meta = now ? [now.episodeLabel, now.facts?.[0], now.timeLabel].filter(Boolean).join(" · ") : "";
  return (
    <Surface backgroundColor="$transparent" borderWidth={0} gap={6} paddingHorizontal={16} paddingTop={16}>
      <View style={{ alignItems: "center", flexDirection: "row", gap: 8 }}>
        <Text density={density} textRole="cardTime" tone="primary">
          {channel.number}
        </Text>
        <Text density={density} flexShrink={1} numberOfLines={1} textRole="cardMeta">
          {channel.name}
        </Text>
        <View style={{ alignItems: "center", flexDirection: "row", gap: 8, marginLeft: "auto" }}>
          {live.mode === "live" ? (
            <>
              <Surface backgroundColor="$guideOnAir" borderRadius={3} borderWidth={0} height={6} width={6} />
              <Text density={density} textRole="cardLabel" tone="live">
                Live
              </Text>
            </>
          ) : (
            <>
              <Text accessibilityLiveRegion="polite" density={density} textRole="cardTime">
                {`${live.mode === "paused" ? "Paused · " : ""}${behindLabel(live.lagSeconds)}`}
              </Text>
              <Pressable
                accessibilityRole="button"
                onPress={onGoLive}
                style={{ justifyContent: "center", minHeight: 44 }}
              >
                <Text density={density} textRole="cardLabel" tone="signal">
                  Go Live
                </Text>
              </Pressable>
            </>
          )}
        </View>
      </View>

      {now ? (
        <>
          <Text density={density} numberOfLines={2} textRole="title">
            {parts?.heading}
            {parts?.episode ? (
              <Text density={density} textRole="headline" tone="secondary">
                {` “${parts.episode}”`}
              </Text>
            ) : null}
          </Text>
          {meta ? (
            <Text density={density} textRole="metadata">
              {meta}
            </Text>
          ) : null}
          {now.progressPercent === undefined ? null : (
            <ProgressTrack marginTop={8} percent={now.progressPercent} tone="artwork" width="100%" />
          )}
          {clockLabel || now.remainingLabel ? (
            <View style={{ flexDirection: "row" }}>
              <Text density={density} textRole="cardTime">
                {clockLabel}
              </Text>
              <Text density={density} marginLeft="auto" textRole="cardTime">
                {now.remainingLabel}
              </Text>
            </View>
          ) : null}
        </>
      ) : null}

      <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
        <Action
          density={density}
          disabled={!canPrevious}
          icon="previous"
          layout="stacked"
          onPress={onPrevious}
          style={{ flex: 1 }}
          tone="secondary"
        >
          Previous
        </Action>
        <Action
          density={density}
          disabled={!canSurf}
          icon="channelDown"
          layout="stacked"
          onPress={onChannelDown}
          style={{ flex: 1 }}
          tone="secondary"
        >
          Channel −
        </Action>
        {live.mode === "paused" ? (
          <Action
            density={density}
            icon="play"
            layout="stacked"
            onPress={onPlay}
            style={{ flex: 1 }}
            tone="secondary"
          >
            Play
          </Action>
        ) : (
          <Action
            density={density}
            icon="pause"
            layout="stacked"
            onPress={onPause}
            style={{ flex: 1 }}
            tone="secondary"
          >
            Pause
          </Action>
        )}
        <Action
          density={density}
          disabled={!canSurf}
          icon="channelUp"
          layout="stacked"
          onPress={onChannelUp}
          style={{ flex: 1 }}
          tone="secondary"
        >
          Channel +
        </Action>
      </View>

      {schedule?.next ? (
        <Text
          density={density}
          numberOfLines={1}
          paddingBottom={4}
          paddingTop={14}
          textRole="caption"
          tone="secondary"
        >
          {`Next ${schedule.next.timeLabel} · `}
          <Text density={density} textRole="caption" tone="primary">
            {schedule.next.title}
          </Text>
        </Text>
      ) : null}

      {favourites.length > 0 ? (
        <Surface backgroundColor="$transparent" borderWidth={0} paddingTop={10}>
          <Text density={density} paddingVertical={6} textRole="label" tracking={0.08}>
            {`FAVORITES · ${favourites.length}`}
          </Text>
          {favourites.map((favourite) => (
            <Pressable
              accessibilityLabel={`Watch ${favourite.channelNumber} ${favourite.channelName}`}
              accessibilityRole="button"
              key={favourite.id}
              onPress={() => onTune(favourite.id)}
            >
              {/* The rule above each row is its own hairline: a Surface's borderWidth resets a top-only border. */}
              <Surface backgroundColor="$borderDecorative" borderWidth={0} height={1} />
              <Surface
                alignItems="center"
                backgroundColor="$transparent"
                borderWidth={0}
                flexDirection="row"
                gap={12}
                height={57}
              >
                <Text density={density} textRole="time" width={32}>
                  {favourite.channelNumber}
                </Text>
                <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
                  {/* The row is a <button> on the web, which centres its text; the mock's rows read left. */}
                  <Text density={density} numberOfLines={1} textAlign="left" textRole="cardTitle">
                    {favourite.channelName}
                  </Text>
                  {favourite.now ? (
                    <Text density={density} numberOfLines={1} textAlign="left" textRole="cardMeta">
                      {programmeLine(favourite.now)}
                    </Text>
                  ) : null}
                </Surface>
                {favourite.now?.remainingLabel ? (
                  <Text density={density} textRole="cardTime">
                    {favourite.now.remainingLabel}
                  </Text>
                ) : null}
              </Surface>
            </Pressable>
          ))}
        </Surface>
      ) : null}
    </Surface>
  );
};

export { WatchingPanel };
