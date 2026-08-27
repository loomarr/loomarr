import { Action, ActivityIndicator, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { Pressable, View } from "react-native";

import { ChannelIdentity, ProgrammeIdentity } from "../identity";
import { TransientOverlay } from "../overlay";
import type { WatchingSurfaceProps } from "./watching-surface.type";
import { behindLabel, playbackMessage } from "./watching-surface-state";
import { TvWatchingSurface } from "./watching-surface-tv";

const TouchWatchingSurface = ({
  chromeVisible = true,
  density,
  loadError,
  numberEntry,
  onChannelDown,
  onChannelUp,
  onDismissControls,
  onGoLive,
  onOpenGuide,
  onOpenSurf,
  onPause,
  onPlay,
  onPrevious,
  onRetry,
  onShowControls,
  player,
  snapshot,
  schedule,
}: WatchingSurfaceProps) => {
  const recoverableFailure = Boolean(loadError) || snapshot.status === "failed";
  const message = playbackMessage(snapshot, loadError);
  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{player}</View>
      {chromeVisible ? (
        <>
          <Pressable
            accessibilityLabel="Show playback controls"
            accessibilityRole="button"
            // Keep one focus target mounted after the transient TV chrome unmounts. Android TV's
            // global event handler only receives D-pad input while the React surface owns focus;
            // without this target, returning from Guide leaves Watching unable to open Surf.
            focusable={density !== "tv" || !snapshot.overlayVisible}
            onPress={onShowControls}
            pointerEvents="auto"
            style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
          />
          {!snapshot.channel && !message ? (
            <View style={{ alignItems: "center", flex: 1, justifyContent: "center" }}>
              <ActivityIndicator
                accessibilityLabel="Loading channels"
                size={density === "tv" ? "tv" : density === "touch" ? "touch" : "default"}
              />
            </View>
          ) : null}
          {numberEntry?.digits ? (
            <Surface left={0} level="overlay" padding="$control" position="absolute" top={0}>
              <Text density={density} textRole="title">
                {`${numberEntry.digits.split("").join(" ")} _`}
              </Text>
              {numberEntry.channelName ? (
                <Text density={density} textRole="metadata">
                  {numberEntry.channelName}
                </Text>
              ) : null}
            </Surface>
          ) : null}
          <TransientOverlay
            autoDismissMs={message || snapshot.status === "tuning" ? undefined : 5_000}
            density={density}
            onDismiss={onDismissControls}
            title="Playback controls"
            visible={snapshot.overlayVisible || Boolean(message)}
          >
            {snapshot.channel ? (
              <ChannelIdentity
                channel={{
                  channelLogoState: "missing",
                  channelName: snapshot.channel.name,
                  channelNumber: String(snapshot.channel.number),
                }}
                density={density}
              />
            ) : null}
            {schedule?.now ? (
              <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
                <ProgrammeIdentity density={density} programme={schedule.now} />
                {schedule.now.progressPercent === undefined ? null : (
                  <ProgressTrack percent={schedule.now.progressPercent} tone="live" width="100%" />
                )}
                {schedule.next ? (
                  <Text density={density} numberOfLines={1} textRole="metadata">
                    {`Next ${schedule.next.timeLabel} · ${schedule.next.title}`}
                  </Text>
                ) : null}
              </Surface>
            ) : null}
            {snapshot.status === "tuning" ? (
              <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
                Tuning…
              </Text>
            ) : null}
            {snapshot.livePlayback ? (
              <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
                {snapshot.livePlayback.mode === "live"
                  ? "Live"
                  : `${snapshot.livePlayback.mode === "paused" ? "Paused · " : ""}${behindLabel(snapshot.livePlayback.lagSeconds)}`}
              </Text>
            ) : null}
            {snapshot.livePlayback?.noticeRevision ? (
              <Text accessibilityLiveRegion="polite" density={density} textRole="body" tone="warning">
                Paused position expired. Returned to live.
              </Text>
            ) : null}
            {message ? (
              <Text
                accessibilityLiveRegion="polite"
                density={density}
                textRole="body"
                tone={recoverableFailure ? "danger" : "muted"}
              >
                {message}
              </Text>
            ) : null}
            <View style={{ flexDirection: "row", gap: density === "tv" ? 16 : 8 }}>
              <Action
                density={density}
                disabled={!snapshot.previousChannelId}
                icon="previous"
                onPress={onPrevious}
                tone="secondary"
              >
                Previous
              </Action>
              <Action
                density={density}
                disabled={snapshot.catalog.length < 2}
                onPress={onChannelDown}
                tone="secondary"
              >
                Channel −
              </Action>
              <Action density={density} icon="guide" onPress={onOpenGuide} tone="secondary">
                Guide
              </Action>
              <Action density={density} icon="channels" onPress={onOpenSurf} tone="secondary">
                Surf
              </Action>
              {snapshot.status === "paused" ? (
                <Action density={density} disabled={!snapshot.channel} onPress={onPlay} tone="primary">
                  Play
                </Action>
              ) : (
                <Action density={density} disabled={!snapshot.channel} onPress={onPause} tone="secondary">
                  Pause
                </Action>
              )}
              {snapshot.livePlayback?.mode && snapshot.livePlayback.mode !== "live" ? (
                <Action density={density} disabled={!snapshot.channel} onPress={onGoLive} tone="secondary">
                  Go Live
                </Action>
              ) : null}
              <Action
                density={density}
                disabled={snapshot.catalog.length < 2}
                onPress={onChannelUp}
                tone="secondary"
              >
                Channel +
              </Action>
              {recoverableFailure ? (
                <Action density={density} onPress={onRetry} tone="primary">
                  Retry
                </Action>
              ) : null}
            </View>
          </TransientOverlay>
        </>
      ) : null}
    </View>
  );
};

const WatchingSurface = (props: WatchingSurfaceProps) =>
  props.density === "tv" ? <TvWatchingSurface {...props} /> : <TouchWatchingSurface {...props} />;

export { WatchingSurface };
