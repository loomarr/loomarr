import { Action, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { Pressable, View } from "react-native";
import { behindLabel } from "../watching-surface-state";
import { useLargeText } from "./use-large-text";
import type { WatchingPanelProps } from "./watching-panel.type";

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
  isFavourite = false,
  live,
  onChannelDown,
  onChannelUp,
  onGoLive,
  onPause,
  onPlay,
  onPrevious,
  onToggleFavourite,
  onTune,
  reconnecting = false,
  schedule,
}: WatchingPanelProps) => {
  const now = schedule?.now;
  const parts = now ? programmeParts(now) : undefined;
  // The mock's line is episode · year · time: one fact (the year, first when known), not all of them.
  const meta = now ? [now.episodeLabel, now.facts?.[0], now.timeLabel].filter(Boolean).join(" · ") : "";
  const largeText = useLargeText();
  const controlStyle = { flex: 1 };
  const controlLayout = largeText ? "inline" : "stacked";
  const controls = [
    <Action
      density={density}
      disabled={!canPrevious}
      icon="previous"
      fitLabel={largeText}
      key="previous"
      layout={controlLayout}
      onPress={onPrevious}
      style={controlStyle}
      tone="secondary"
    >
      Previous
    </Action>,
    <Action
      density={density}
      disabled={!canSurf}
      icon="channelDown"
      fitLabel={largeText}
      key="channelDown"
      layout={controlLayout}
      onPress={onChannelDown}
      style={controlStyle}
      tone="secondary"
    >
      Channel −
    </Action>,
    live.mode === "paused" ? (
      <Action
        density={density}
        icon="play"
        fitLabel={largeText}
        key="play"
        layout={controlLayout}
        onPress={onPlay}
        style={controlStyle}
        tone="secondary"
      >
        Play
      </Action>
    ) : (
      <Action
        density={density}
        icon="pause"
        fitLabel={largeText}
        key="pause"
        layout={controlLayout}
        onPress={onPause}
        style={controlStyle}
        tone="secondary"
      >
        Pause
      </Action>
    ),
    <Action
      density={density}
      disabled={!canSurf}
      icon="channelUp"
      fitLabel={largeText}
      key="channelUp"
      layout={controlLayout}
      onPress={onChannelUp}
      style={controlStyle}
      tone="secondary"
    >
      Channel +
    </Action>,
  ];
  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap={6} paddingHorizontal={16} paddingTop={16}>
      {/* At an accessibility text size the Go Live group wraps under the channel instead of squeezing
          the name out of the row, and the name wraps rather than clips. Wrapping is off otherwise: a
          wrapping row would drop the Live group below a long name that shrinks to fit today. */}
      <View
        style={{
          alignItems: "center",
          columnGap: 8,
          flexDirection: "row",
          flexWrap: largeText ? "wrap" : "nowrap",
          rowGap: 4,
        }}
      >
        <Text density={density} textRole="cardTime" tone="primary">
          {channel.number}
        </Text>
        <Text density={density} flexShrink={1} numberOfLines={largeText ? undefined : 1} textRole="cardMeta">
          {channel.name}
        </Text>
        {onToggleFavourite ? (
          // The Guide's filter writes this same glyph ("★ Favorites"). The hit area is 44pt either way.
          <Pressable
            accessibilityLabel={isFavourite ? "Remove from Favorites" : "Add to Favorites"}
            accessibilityRole="button"
            aria-selected={isFavourite}
            hitSlop={{ bottom: 8, left: 4, right: 4, top: 8 }}
            onPress={onToggleFavourite}
            style={{ alignItems: "center", justifyContent: "center", minHeight: 44, minWidth: 32 }}
          >
            <Text density={density} textRole="cardLabel" tone={isFavourite ? "primary" : "secondary"}>
              {isFavourite ? "★" : "☆"}
            </Text>
          </Pressable>
        ) : null}
        <View
          style={{
            alignItems: "center",
            flexDirection: "row",
            gap: 8,
            marginLeft: "auto",
            // A wrapped line is only as wide as the row: let the group wrap too, not run off the screen.
            ...(largeText ? { flexShrink: 1, flexWrap: "wrap", maxWidth: "100%" } : {}),
          }}
        >
          {live.mode === "live" ? (
            <>
              <Surface backgroundColor="guideOnAir" borderRadius={3} borderWidth={0} height={6} width={6} />
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

      {reconnecting ? (
        <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
          Reconnecting…
        </Text>
      ) : null}

      {now ? (
        <>
          <Text density={density} numberOfLines={largeText ? undefined : 2} textRole="title">
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

      {largeText ? (
        // At an accessibility text size four tiles cannot share a row, and the stacked tile's label is
        // one line (it would read "Pre", "Ch"): two rows of two, each an icon beside a label that wraps
        // rather than clips, as Control Center lays out its controls.
        <View style={{ gap: 8, marginTop: 8 }}>
          <View style={{ flexDirection: "row", gap: 8 }}>{controls.slice(0, 2)}</View>
          <View style={{ flexDirection: "row", gap: 8 }}>{controls.slice(2)}</View>
        </View>
      ) : (
        <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>{controls}</View>
      )}

      {schedule?.next ? (
        <Text
          density={density}
          numberOfLines={largeText ? undefined : 1}
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
        <Surface backgroundColor="transparent" borderWidth={0} paddingTop={10}>
          <Text density={density} paddingVertical={6} textRole="label" tracking={0.08}>
            {`FAVORITES · ${favourites.length}`}
          </Text>
          {favourites.map((favourite) => (
            <Pressable accessibilityRole="button" key={favourite.id} onPress={() => onTune(favourite.id)}>
              {/* The rule above each row is its own hairline: a Surface's borderWidth resets a top-only border. */}
              <Surface backgroundColor="borderDecorative" borderWidth={0} height={1} />
              <Surface
                alignItems="center"
                backgroundColor="transparent"
                borderWidth={0}
                flexDirection="row"
                gap={12}
                {...(largeText ? { minHeight: 57, paddingVertical: 8 } : { height: 57 })}
              >
                <Text density={density} textRole="time" width={32}>
                  {favourite.channelNumber}
                </Text>
                <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
                  {/* The row is a <button> on the web, which centres its text; the mock's rows read left. */}
                  <Text
                    density={density}
                    numberOfLines={largeText ? undefined : 1}
                    textAlign="left"
                    textRole="cardTitle"
                  >
                    {favourite.channelName}
                  </Text>
                  {favourite.now ? (
                    <Text
                      density={density}
                      numberOfLines={largeText ? undefined : 1}
                      textAlign="left"
                      textRole="cardMeta"
                    >
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
