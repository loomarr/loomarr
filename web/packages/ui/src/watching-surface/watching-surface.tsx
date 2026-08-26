import type { Density } from "@loomarr/design-system";
import { Action, ActivityIndicator, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";
import { Pressable, View } from "react-native";

import { ChannelIdentity, ProgrammeIdentity, type ProgrammeIdentityData } from "../identity";
import { TransientOverlay } from "../overlay";

interface ChannelNumberEntry {
  channelName?: string;
  digits: string;
}

interface WatchingProgrammeData extends ProgrammeIdentityData {
  progressPercent?: number;
}

interface WatchingScheduleData {
  next?: Pick<ProgrammeIdentityData, "timeLabel" | "title">;
  now?: WatchingProgrammeData;
}

interface WatchingSurfaceProps {
  chromeVisible?: boolean;
  density: Density;
  loadError?: string;
  numberEntry?: ChannelNumberEntry;
  onChannelDown: () => void;
  onChannelUp: () => void;
  onDismissControls: () => void;
  onGoLive: () => void;
  onOpenGuide: () => void;
  onOpenSurf: () => void;
  onPause: () => void;
  onPlay: () => void;
  onPrevious: () => void;
  onRetry: () => void;
  onShowControls: () => void;
  player: ReactNode;
  snapshot: PlayerSnapshot;
  schedule?: WatchingScheduleData;
}

const WatchingSurface = ({
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
  const message =
    loadError ??
    (snapshot.status === "empty"
      ? "No playable channels are on this Loomarr yet."
      : snapshot.status === "idle"
        ? "Choose a channel from the Guide or Surf."
        : snapshot.status === "failed"
          ? snapshot.error
          : undefined);
  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{player}</View>
      {chromeVisible ? (
        <>
          <Pressable
            accessibilityLabel="Show playback controls"
            accessibilityRole="button"
            focusable={density !== "tv"}
            onPress={onShowControls}
            pointerEvents={density === "tv" ? "none" : "auto"}
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
              <Action density={density} disabled={!snapshot.channel} onPress={onGoLive} tone="secondary">
                Go Live
              </Action>
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

export type { ChannelNumberEntry, WatchingProgrammeData, WatchingScheduleData, WatchingSurfaceProps };
export { WatchingSurface };
