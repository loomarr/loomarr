import { Action, ActivityIndicator, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { useEffect } from "react";
import { Pressable, View } from "react-native";
import type { WatchingSurfaceProps } from "../watching-surface.type";
import { behindLabel, playbackMessage } from "../watching-surface-state";

const RemoteHint = ({ children }: { children: string }) => (
  <Text density="tv" textRole="metadata" tone="muted">
    {children}
  </Text>
);

const TvWatchingSurface = ({
  chromeVisible = true,
  loadError,
  numberEntry,
  onDismissControls,
  onGoLive,
  onPlay,
  onRetry,
  onShowControls,
  player,
  snapshot,
  schedule,
}: WatchingSurfaceProps) => {
  const message = playbackMessage(snapshot, loadError);
  const recoverableFailure = Boolean(loadError) || snapshot.status === "failed";
  const overlayVisible = snapshot.overlayVisible || Boolean(message) || snapshot.status === "tuning";

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
            accessibilityLabel="Show playback controls"
            accessibilityRole="button"
            focusable={!snapshot.overlayVisible}
            onPress={onShowControls}
            pointerEvents="auto"
            style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
          />

          {!snapshot.channel && !message ? (
            <View style={{ alignItems: "center", flex: 1, justifyContent: "center" }}>
              <ActivityIndicator accessibilityLabel="Loading channels" size="tv" />
            </View>
          ) : null}

          {numberEntry?.digits ? (
            <Surface
              gap={4}
              left={48}
              level="overlay"
              minWidth={176}
              padding="$control"
              position="absolute"
              top={48}
            >
              <Text density="tv" textRole="title">
                {`${numberEntry.digits.split("").join(" ")} _`}
              </Text>
              {numberEntry.channelName ? (
                <Text density="tv" numberOfLines={1} textRole="metadata">
                  {numberEntry.channelName}
                </Text>
              ) : null}
            </Surface>
          ) : null}

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

export { TvWatchingSurface };
