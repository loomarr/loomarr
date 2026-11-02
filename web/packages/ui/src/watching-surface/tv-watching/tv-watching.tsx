import { Surface, Text } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { useEffect, useState } from "react";
import { Pressable, View } from "react-native";

import { ChannelSwitchOverlay } from "../../channel-switch-overlay";
import { StatePanel } from "../../state-panel";
import { PlayerChrome } from "../player-chrome/player-chrome";
import { PlayerCover } from "../player-chrome/player-cover";
import type { ChannelNumberEntry, TvWatchingProps } from "./tv-watching.type";

// TvWatching — Watching on Android TV and Apple TV (#2141): the one picture under the §9 chrome every
// player wears, at TV density. Over a bare picture a focusable layer takes OK for the app's remote map
// (#2088 decision 15); the controls, mounted only while they show, take the focus on Play/Pause. A tune
// the viewer asked for is covered by the channel-switch overlay, and typed digits read top left.

/** Viewer-initiated tunes only: a recovery retry or the boot-time catalog tune has no "switch" to cover. */
const SWITCH_REASONS: ReadonlySet<PlayerSnapshot["tuneReason"]> = new Set([
  "channel",
  "number",
  "previous",
  "step",
]);

/** Why there is no picture when no channel is tuned: an empty server, or nothing chosen yet. */
const nothingMessage = (snapshot: PlayerSnapshot) =>
  snapshot.status === "empty"
    ? "No playable channels are on this Loomarr yet."
    : snapshot.status === "idle"
      ? "Choose a channel from the Guide or Surf."
      : snapshot.status === "failed"
        ? snapshot.error
        : undefined;

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

const NumberEntry = ({ numberEntry }: { numberEntry?: ChannelNumberEntry }) => {
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

const fill = { bottom: 0, left: 0, position: "absolute", right: 0, top: 0 } as const;

const TvWatching = ({
  chrome,
  chromeVisible,
  controlsShown,
  loading,
  loadError,
  numberEntry,
  onBarePress,
  onChangeServer,
  onLeave,
  onRetry,
  onSwitchShown,
  player,
  snapshot,
}: TvWatchingProps) => {
  const channel = snapshot.channel;
  const channelLabel = channel ? `CH ${channel.number} · ${channel.name}` : "";
  const switching =
    !loading &&
    snapshot.status === "tuning" &&
    !snapshot.reconnecting &&
    SWITCH_REASONS.has(snapshot.tuneReason);
  // Nothing to play: the catalog failed, is empty, or no channel is tuned yet.
  const nothing = loading || loadError || channel ? undefined : nothingMessage(snapshot);
  const channelFailed = !loadError && snapshot.status === "failed";
  const covered = loading || Boolean(loadError) || channelFailed || nothing !== undefined;

  const body = loading ? (
    <View style={[fill, { justifyContent: "center" }]}>
      <StatePanel density="tv" kind="loading" title="Loading channels" />
    </View>
  ) : loadError ? (
    <PlayerCover
      channelLabel=""
      density="tv"
      kind="error"
      leaveLabel="Change server"
      message={loadError}
      onLeave={onChangeServer}
      onRetry={onRetry}
      retryLabel="Retry"
      title="Can't load channels"
    />
  ) : channelFailed ? (
    <PlayerCover
      channelLabel={channelLabel}
      density="tv"
      kind="error"
      leaveLabel="Guide"
      message={snapshot.error}
      onLeave={onLeave}
      onRetry={onRetry}
      title="Can't play this channel"
    />
  ) : nothing !== undefined ? (
    <View style={[fill, { justifyContent: "center" }]}>
      <StatePanel
        action={snapshot.status === "idle" ? { label: "Guide", onPress: onLeave } : undefined}
        density="tv"
        kind="empty"
        title={nothing}
      />
    </View>
  ) : chrome ? (
    <PlayerChrome {...chrome} density="tv" visible={controlsShown && snapshot.status !== "tuning"} />
  ) : null;

  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={fill}>{player}</View>
      {/* Mounted from the first channel on, hidden, so a switch key only has to show it (#1781). */}
      {channel ? (
        <ChannelSwitchOverlay
          channel={{ channelName: channel.name, channelNumber: String(channel.number) }}
          onShown={onSwitchShown}
          stillUri={snapshot.stillUri}
          visible={chromeVisible && switching}
        />
      ) : null}
      {chromeVisible ? (
        <>
          {/* OK over the bare picture. Gone while the controls or a cover hold the focus. */}
          {controlsShown || covered ? null : (
            <Pressable
              accessibilityLabel="Show the controls"
              accessibilityRole="button"
              hasTVPreferredFocus
              onPress={onBarePress}
              style={fill}
            />
          )}
          {body}
          <NumberEntry numberEntry={numberEntry} />
        </>
      ) : null}
    </View>
  );
};

export { TvWatching };
