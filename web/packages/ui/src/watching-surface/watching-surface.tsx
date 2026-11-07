import { Action, ActivityIndicator, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { useCallback, useEffect, useRef, useState } from "react";
import { Pressable, View } from "react-native";

import { ChannelSwitchOverlay } from "../channel-switch-overlay";
import { ChannelIdentity } from "../identity";
import type { WatchingSurfaceProps } from "./watching-surface.type";
import { behindLabel, playbackMessage } from "./watching-surface-state";

const LoadingChannels = () => (
  <View style={{ alignItems: "center", flex: 1, justifyContent: "center" }}>
    {/* The indicator aligns itself to the start (so it never stretches); this wrapper carries it to the centre. */}
    <View style={{ alignSelf: "center" }}>
      <ActivityIndicator accessibilityLabel="Loading channels" size="tv" />
    </View>
  </View>
);

/** "auto-tunes in 1.2 s" (#1659 map 5a), ticking down to the readout's own expiry. */
const useAutoTuneCountdownLabel = (expiresAtMs: number | undefined): string | undefined => {
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    if (expiresAtMs === undefined) return undefined;
    setNowMs(Date.now());
    const interval = setInterval(() => setNowMs(Date.now()), 100);
    return () => clearInterval(interval);
  }, [expiresAtMs]);
  if (expiresAtMs === undefined) return undefined;
  const remainingSeconds = Math.max(0, (expiresAtMs - nowMs) / 1000);
  return `auto-tunes in ${remainingSeconds.toFixed(1)} s`;
};

const NumberEntry = ({ numberEntry }: Pick<WatchingSurfaceProps, "numberEntry">) => {
  const countdown = useAutoTuneCountdownLabel(numberEntry?.digits ? numberEntry.expiresAtMs : undefined);
  if (!numberEntry?.digits) return null;
  return (
    <Surface
      alignItems="center"
      backgroundColor="surfaceEntry"
      borderRadius={8}
      flexDirection="row"
      gap={16}
      left={48}
      paddingHorizontal={20}
      paddingVertical={12}
      position="absolute"
      top={48}
    >
      {/* The direct-tune digits are a display numeral, which keeps amber (#1909 Q4). */}
      <Text density="tv" textRole="channelNumber" tone="signal">
        {`${numberEntry.digits.split("").join(" ")} _`}
      </Text>
      {numberEntry.channelName ? (
        <Text density="tv" numberOfLines={1} textRole="reading" tone="muted">
          {numberEntry.channelName}
        </Text>
      ) : null}
      {countdown ? (
        <Text accessibilityLiveRegion="polite" density="tv" textRole="metadata" tone="muted">
          {countdown}
        </Text>
      ) : null}
    </Surface>
  );
};

/** Viewer-initiated tunes only: a recovery retry or the boot-time catalog tune has no "switch" to cover. */
const SWITCH_REASONS: ReadonlySet<PlayerSnapshot["tuneReason"]> = new Set([
  "channel",
  "number",
  "previous",
  "step",
]);

const WatchingSurface = ({
  channelDigits = true,
  chromeVisible = true,
  controlsActivityKey = 0,
  controlsVisible = true,
  loading = false,
  loadError,
  numberEntry,
  onChangeServer,
  onDismissControls,
  onGoLive,
  onOpenGuide,
  onPlay,
  onRetry,
  onSwitchShown,
  player,
  schedule,
  snapshot,
}: WatchingSurfaceProps) => {
  const message = loading ? undefined : playbackMessage(snapshot, loadError);
  const recoverableFailure = Boolean(loadError) || snapshot.status === "failed";
  const overlayVisible = !loading && (controlsVisible || Boolean(message) || snapshot.status === "tuning");
  const activityKeyRef = useRef(controlsActivityKey);
  const dismissControlsRef = useRef(onDismissControls);
  const switching =
    !loading &&
    snapshot.status === "tuning" &&
    !snapshot.reconnecting &&
    SWITCH_REASONS.has(snapshot.tuneReason);

  useEffect(() => {
    activityKeyRef.current = controlsActivityKey;
  }, [controlsActivityKey]);
  useEffect(() => {
    dismissControlsRef.current = onDismissControls;
  }, [onDismissControls]);
  const dismissControlsForActivity = useCallback(() => {
    if (activityKeyRef.current !== controlsActivityKey) return;
    dismissControlsRef.current();
  }, [controlsActivityKey]);

  useEffect(() => {
    if (!overlayVisible || message || snapshot.status === "tuning") return undefined;
    const timeout = setTimeout(dismissControlsForActivity, 5_000);
    return () => clearTimeout(timeout);
  }, [dismissControlsForActivity, message, overlayVisible, snapshot.status]);

  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{player}</View>
      {/* Mounted from the first channel on, hidden, so a switch key only has to show it (#1781). */}
      {snapshot.channel ? (
        <ChannelSwitchOverlay
          channel={{ channelName: snapshot.channel.name, channelNumber: String(snapshot.channel.number) }}
          onShown={onSwitchShown}
          stillUri={snapshot.stillUri}
          visible={chromeVisible && switching}
        />
      ) : null}
      {chromeVisible ? (
        <>
          {loading ? null : (
            <Pressable
              accessibilityLabel="Open programme guide"
              accessibilityRole="button"
              focusable={snapshot.status !== "paused" && !recoverableFailure}
              hasTVPreferredFocus={snapshot.status !== "paused" && !recoverableFailure}
              onPress={onOpenGuide}
              pointerEvents="auto"
              style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
            />
          )}
          {loading ? <LoadingChannels /> : null}
          <NumberEntry numberEntry={numberEntry} />
          {overlayVisible ? (
            <>
              {/* NumberEntry owns the top-left while digits are typed; the chip returns on tune or cancel
                  (tv-auto-tune-setting.html state 5, map 5a). */}
              {snapshot.channel && !numberEntry?.digits ? (
                <Surface
                  alignItems="center"
                  backgroundColor="surfaceIdentity"
                  borderRadius={8}
                  borderWidth={1}
                  flexDirection="row"
                  gap={12}
                  paddingHorizontal={16}
                  paddingVertical={8}
                  position="absolute"
                  right={48}
                  top={48}
                >
                  <ChannelIdentity
                    channel={{
                      channelLogoState: "missing",
                      channelName: snapshot.channel.name.toUpperCase(),
                      channelNumber: String(snapshot.channel.number),
                    }}
                    density="tv"
                  />
                  {snapshot.status === "playing" ? (
                    <Surface
                      backgroundColor="stateSuccess"
                      borderRadius="round"
                      borderWidth={0}
                      height={8}
                      width={8}
                    />
                  ) : null}
                </Surface>
              ) : null}
              <Surface
                backgroundColor="surfaceChrome"
                borderRadius={0}
                borderWidth={0}
                bottom={0}
                gap={0}
                left={0}
                paddingTop={24}
                position="absolute"
                right={0}
              >
                {schedule?.now ? (
                  <Surface backgroundColor="transparent" borderWidth={0} gap={0}>
                    <Surface
                      alignItems="center"
                      backgroundColor="transparent"
                      borderWidth={0}
                      flexDirection="row"
                      gap={12}
                      paddingHorizontal={64}
                    >
                      <Text density="tv" flex={1} numberOfLines={1} textRole="headline">
                        {schedule.now.title}
                      </Text>
                      {schedule.now.episodeLabel ? (
                        <Text density="tv" textRole="data" tone="muted">
                          {schedule.now.episodeLabel}
                        </Text>
                      ) : null}
                      {schedule.now.facts?.[0] ? (
                        <Text density="tv" textRole="time" tone="muted">
                          {schedule.now.facts[0]}
                        </Text>
                      ) : null}
                      <Text density="tv" textRole="time" tone="muted">
                        {schedule.now.timeLabel}
                      </Text>
                    </Surface>
                    {schedule.now.progressPercent === undefined ? null : (
                      <ProgressTrack
                        height={5}
                        marginTop={12}
                        percent={schedule.now.progressPercent}
                        width="100%"
                      />
                    )}
                  </Surface>
                ) : null}
                {snapshot.status === "tuning" ? (
                  <Text
                    accessibilityLiveRegion="polite"
                    density="tv"
                    marginHorizontal={64}
                    marginTop={12}
                    textRole="metadata"
                  >
                    {snapshot.reconnecting ? "Reconnecting…" : "Tuning…"}
                  </Text>
                ) : null}
                {message ? (
                  <Text
                    accessibilityLiveRegion="polite"
                    density="tv"
                    marginHorizontal={64}
                    marginTop={12}
                    textRole="body"
                    tone={recoverableFailure ? "danger" : "muted"}
                  >
                    {message}
                  </Text>
                ) : null}
                {snapshot.livePlayback?.noticeRevision ? (
                  <Text
                    accessibilityLiveRegion="polite"
                    density="tv"
                    marginHorizontal={64}
                    marginTop={12}
                    textRole="body"
                    tone="warning"
                  >
                    Paused position expired. Returned to live.
                  </Text>
                ) : null}
                <Surface
                  alignItems="center"
                  backgroundColor="transparent"
                  borderWidth={0}
                  flexDirection="row"
                  paddingBottom={16}
                  paddingHorizontal={64}
                  paddingTop={12}
                >
                  <Text density="tv" flex={1} numberOfLines={1} textRole="reading" tone="muted">
                    {schedule?.next
                      ? `Up next · ${schedule.next.timeLabel} — ${schedule.next.title}`
                      : "No later programme in this guide window"}
                  </Text>
                  {snapshot.livePlayback && snapshot.livePlayback.mode !== "live" ? (
                    <Text density="tv" textRole="metadata">
                      {`${snapshot.livePlayback.mode === "paused" ? "Paused · " : ""}${behindLabel(snapshot.livePlayback.lagSeconds)}`}
                    </Text>
                  ) : null}
                  <Text density="tv" marginLeft={24} textRole="metadata" tone="muted">
                    {channelDigits
                      ? "▲▼ tune · ◀ channels · 0–9 jump · OK guide"
                      : "▲▼ tune · ◀ channels · OK guide"}
                  </Text>
                </Surface>
                {snapshot.status === "paused" || recoverableFailure ? (
                  <Surface
                    backgroundColor="transparent"
                    borderWidth={0}
                    flexDirection="row"
                    gap="control"
                    paddingBottom={16}
                    paddingHorizontal={64}
                  >
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
                      // The guide surface above stops being focusable on a failure, and tvOS leaves
                      // focus where it was unless something claims it: Retry does, so OK retries.
                      <Action density="tv" hasTVPreferredFocus onPress={onRetry} tone="primary">
                        Retry
                      </Action>
                    ) : null}
                    {loadError && onChangeServer ? (
                      <Action density="tv" onPress={onChangeServer} tone="secondary">
                        Change server
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

export { WatchingSurface };
