import { Action, ActivityIndicator, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { useEffect } from "react";
import { Pressable, View } from "react-native";

import { ChannelIdentity, ProgrammeIdentity } from "../identity";
import { TransientOverlay } from "../overlay";
import type { WatchingSurfaceProps } from "./watching-surface.type";
import { behindLabel, playbackMessage } from "./watching-surface-state";

const RemoteHint = ({ children }: { children: string }) => (
  <Text density="tv" textRole="metadata" tone="muted">
    {children}
  </Text>
);

const LoadingChannels = ({ density }: Pick<WatchingSurfaceProps, "density">) => (
  <View style={{ alignItems: "center", flex: 1, justifyContent: "center" }}>
    <ActivityIndicator
      accessibilityLabel="Loading channels"
      size={density === "tv" ? "tv" : density === "touch" ? "touch" : "default"}
    />
  </View>
);

const NumberEntry = ({ density, numberEntry }: Pick<WatchingSurfaceProps, "density" | "numberEntry">) =>
  numberEntry?.digits ? (
    <Surface
      gap={4}
      left={density === "tv" ? 48 : 0}
      level="overlay"
      minWidth={density === "tv" ? 176 : undefined}
      padding="$control"
      position="absolute"
      top={density === "tv" ? 48 : 0}
    >
      <Text density={density} textRole="title">
        {`${numberEntry.digits.split("").join(" ")} _`}
      </Text>
      {numberEntry.channelName ? (
        <Text density={density} numberOfLines={1} textRole="metadata">
          {numberEntry.channelName}
        </Text>
      ) : null}
    </Surface>
  ) : null;

const TouchWatchingSurface = ({
  chromeVisible = true,
  controlsVisible = true,
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
  schedule,
  snapshot,
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
            onPress={onShowControls}
            pointerEvents="auto"
            style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
          />
          {!snapshot.channel && !message ? <LoadingChannels density={density} /> : null}
          <NumberEntry density={density} numberEntry={numberEntry} />
          <TransientOverlay
            autoDismissMs={message || snapshot.status === "tuning" ? undefined : 5_000}
            density={density}
            onDismiss={onDismissControls}
            title="Playback controls"
            visible={controlsVisible || Boolean(message) || snapshot.status === "tuning"}
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
            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: density === "touch" ? 8 : 6 }}>
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

const TvWatchingSurface = ({
  chromeVisible = true,
  controlsVisible = true,
  loadError,
  numberEntry,
  onDismissControls,
  onGoLive,
  onOpenGuide,
  onPlay,
  onRetry,
  player,
  schedule,
  snapshot,
}: WatchingSurfaceProps) => {
  const message = playbackMessage(snapshot, loadError);
  const recoverableFailure = Boolean(loadError) || snapshot.status === "failed";
  const overlayVisible = controlsVisible || Boolean(message) || snapshot.status === "tuning";

  useEffect(() => {
    if (!overlayVisible || message || snapshot.status === "tuning") return undefined;
    const timeout = setTimeout(onDismissControls, 5_000);
    return () => clearTimeout(timeout);
  }, [message, onDismissControls, overlayVisible, snapshot.status]);

  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{player}</View>
      {chromeVisible ? (
        <>
          <Pressable
            accessibilityLabel="Open programme guide"
            accessibilityRole="button"
            focusable={snapshot.status !== "paused" && !recoverableFailure}
            hasTVPreferredFocus={snapshot.status !== "paused" && !recoverableFailure}
            onPress={onOpenGuide}
            pointerEvents="auto"
            style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
          />
          {!snapshot.channel && !message ? <LoadingChannels density="tv" /> : null}
          <NumberEntry density="tv" numberEntry={numberEntry} />
          {overlayVisible ? (
            <>
              {snapshot.channel ? (
                <Surface
                  alignItems="center"
                  flexDirection="row"
                  gap="$inline"
                  level="overlay"
                  paddingHorizontal="$control"
                  paddingVertical="$inline"
                  position="absolute"
                  right={48}
                  top={48}
                >
                  <Text density="tv" textRole="channelNumber">
                    {snapshot.channel.number}
                  </Text>
                  <Text density="tv" numberOfLines={1} textRole="label">
                    {snapshot.channel.name}
                  </Text>
                </Surface>
              ) : null}
              <Surface
                backgroundColor="$surfaceOverlay"
                borderRadius={0}
                borderWidth={0}
                bottom={0}
                gap="$control"
                left={0}
                paddingBottom={40}
                paddingHorizontal={48}
                paddingTop="$control"
                position="absolute"
                right={0}
              >
                {schedule?.now ? (
                  <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
                    <Surface
                      alignItems="flex-end"
                      backgroundColor="$transparent"
                      borderWidth={0}
                      flexDirection="row"
                      gap="$control"
                    >
                      <Text density="tv" flex={1} numberOfLines={1} textRole="title">
                        {schedule.now.title}
                      </Text>
                      <Text density="tv" textRole="time">
                        {[schedule.now.timeLabel, schedule.now.episodeLabel].filter(Boolean).join(" · ")}
                      </Text>
                    </Surface>
                    {schedule.now.facts?.length ? (
                      <Text density="tv" numberOfLines={1} textRole="metadata">
                        {schedule.now.facts.join(" · ")}
                      </Text>
                    ) : null}
                    {schedule.now.progressPercent === undefined ? null : (
                      <ProgressTrack percent={schedule.now.progressPercent} tone="live" width="100%" />
                    )}
                  </Surface>
                ) : null}
                {snapshot.status === "tuning" ? (
                  <Text accessibilityLiveRegion="polite" density="tv" textRole="metadata">
                    Tuning…
                  </Text>
                ) : null}
                {message ? (
                  <Text
                    accessibilityLiveRegion="polite"
                    density="tv"
                    textRole="body"
                    tone={recoverableFailure ? "danger" : "muted"}
                  >
                    {message}
                  </Text>
                ) : null}
                {snapshot.livePlayback?.noticeRevision ? (
                  <Text accessibilityLiveRegion="polite" density="tv" textRole="body" tone="warning">
                    Paused position expired. Returned to live.
                  </Text>
                ) : null}
                <Surface
                  alignItems="center"
                  backgroundColor="$transparent"
                  borderWidth={0}
                  flexDirection="row"
                  gap="$control"
                >
                  <Text density="tv" flex={1} numberOfLines={1} textRole="metadata">
                    {schedule?.next ? `Next ${schedule.next.timeLabel} · ${schedule.next.title}` : " "}
                  </Text>
                  {snapshot.livePlayback?.mode === "live" ? (
                    <Text density="tv" textRole="metadata" tone="live">
                      Live
                    </Text>
                  ) : snapshot.livePlayback ? (
                    <Text density="tv" textRole="metadata">
                      {`${snapshot.livePlayback.mode === "paused" ? "Paused · " : ""}${behindLabel(snapshot.livePlayback.lagSeconds)}`}
                    </Text>
                  ) : null}
                  <RemoteHint>Up/Down tune</RemoteHint>
                  <RemoteHint>Left Surf</RemoteHint>
                  <RemoteHint>0–9 jump</RemoteHint>
                  <RemoteHint>OK Guide</RemoteHint>
                </Surface>
                {snapshot.status === "paused" || recoverableFailure ? (
                  <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$control">
                    {snapshot.status === "paused" ? (
                      <Action density="tv" disabled={!snapshot.channel} onPress={onPlay} tone="primary">
                        Play
                      </Action>
                    ) : null}
                    {snapshot.livePlayback?.mode && snapshot.livePlayback.mode !== "live" ? (
                      <Action density="tv" disabled={!snapshot.channel} onPress={onGoLive} tone="secondary">
                        Go Live
                      </Action>
                    ) : null}
                    {recoverableFailure ? (
                      <Action density="tv" onPress={onRetry} tone="primary">
                        Retry
                      </Action>
                    ) : null}
                  </Surface>
                ) : null}
              </Surface>
            </>
          ) : null}
        </>
      ) : null}
    </View>
  );
};

const WatchingSurface = (props: WatchingSurfaceProps) =>
  props.density === "tv" ? <TvWatchingSurface {...props} /> : <TouchWatchingSurface {...props} />;

export { WatchingSurface };
