import { LoadingStrip, playerChrome, Text, ThemeScope } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { useCallback, useEffect, useRef, useState } from "react";
import { Pressable, View } from "react-native";

import { ChannelSwitchOverlay } from "../channel-switch-overlay";
import {
  PlayerAudio,
  PlayerChrome,
  PlayerCover,
  type PlayerLiveState,
  PlayerMenu,
  PlayerMore,
} from "./player-chrome";
import { PlayerReadout } from "./player-chrome/player-readout";
import type { WatchingSurfaceProps } from "./watching-surface.type";

// Watching on a television (frontend-design §9.7): the picture, with the J chrome drawn over it while
// the controls are up — the band with its film strip bottom left, the keys bottom right — and the
// covers in its place when there is nothing to play. Focus starts on the strip's block on air, and
// OK there opens the Guide, as OK does on the bare picture.

const LIVE: PlayerLiveState = { lagSeconds: 0, mode: "live" };
// A television keeps 5% clear at every edge (overscan): 48 × 27 at 960 × 540.
const EDGE = { side: 48, top: 28 } as const;

const Centred = ({ label }: { label: string }) => (
  <View
    pointerEvents="none"
    style={{
      alignItems: "center",
      bottom: 0,
      justifyContent: "center",
      left: 0,
      position: "absolute",
      right: 0,
      top: 0,
    }}
  >
    {/* The page wait is the colour-bar strip (#2789); this wrapper carries it to the centre. */}
    <View style={{ alignSelf: "center" }}>
      <LoadingStrip accessibilityLabel={label} density="tv" width={240} />
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

/** Viewer-initiated tunes only: a recovery retry or the boot-time catalog tune has no "switch" to cover. */
const SWITCH_REASONS: ReadonlySet<PlayerSnapshot["tuneReason"]> = new Set([
  "channel",
  "number",
  "previous",
  "step",
]);

/** The cover that takes the picture's place, when there is nothing to play. */
const coverOf = (snapshot: PlayerSnapshot, loadError: string | undefined) => {
  if (loadError) return { kind: "error", message: loadError, title: "The channels didn't load" } as const;
  if (snapshot.status === "failed")
    return { kind: "error", message: snapshot.error, title: "The stream stopped." } as const;
  if (snapshot.status === "empty")
    return {
      kind: "off-air",
      message: "No playable channels are on this Loomarr yet.",
      title: "No channels yet",
    } as const;
  if (snapshot.status === "idle")
    return {
      kind: "off-air",
      message: "Choose a channel from the Guide or Surf.",
      title: "Nothing on yet",
    } as const;
  return undefined;
};

const WatchingSurface = ({
  audioOptions,
  chrome,
  chromeVisible = true,
  controlsActivityKey = 0,
  controlsVisible = true,
  formatTime,
  layer,
  loading = false,
  loadError,
  numberEntry,
  onChangeServer,
  onDismissControls,
  onGoLive,
  onLayer,
  onOpenGuide,
  onOpenSurf,
  onRetry,
  onSeek,
  onSwitchShown,
  onTuneChannel,
  playheadMs,
  player,
  snapshot,
}: WatchingSurfaceProps) => {
  const cover = loading ? undefined : coverOf(snapshot, loadError);
  const tuning = !loading && snapshot.status === "tuning";
  const switching = tuning && !snapshot.reconnecting && SWITCH_REASONS.has(snapshot.tuneReason);
  // The controls are up for activity and through a tune; a cover stands in for them.
  const shown = !loading && !cover && Boolean(chrome) && (controlsVisible || tuning);
  const typing = Boolean(numberEntry?.digits);
  const countdown = useAutoTuneCountdownLabel(typing ? numberEntry?.expiresAtMs : undefined);
  const previous = snapshot.catalog.find(({ id }) => id === snapshot.previousChannelId);
  const strip = chrome && chrome.segments.length > 0;
  const live = snapshot.livePlayback ?? LIVE;
  const startOverMs = chrome?.startOverMs;
  // OK opens the Guide (playback-clients). While the controls are up, focus rests in them (the strip's
  // block on air, whose OK opens the Guide too); otherwise on the picture itself.
  const pictureFocus = !loading && cover?.kind !== "error" && !(shown && (strip || previous));

  const activityKeyRef = useRef(controlsActivityKey);
  const dismissControlsRef = useRef(onDismissControls);
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
  // The controls clear five seconds after the last handled key (playback-clients), not on a tune, and
  // not while a menu (Audio or More) is open: it waits for a choice, or for Menu or Back.
  const menuOpen = layer === "audio" || layer === "more";
  useEffect(() => {
    if (!shown || tuning || menuOpen) return undefined;
    const timeout = setTimeout(dismissControlsForActivity, 5_000);
    return () => clearTimeout(timeout);
  }, [dismissControlsForActivity, menuOpen, shown, tuning]);

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
        <ThemeScope theme="dark">
          {pictureFocus ? (
            // Mounted only while it is where focus belongs: as the controls come up it goes, and the
            // strip's block on air claims focus; as they clear it comes back and claims it again.
            <Pressable
              accessibilityLabel="Open programme guide"
              accessibilityRole="button"
              hasTVPreferredFocus
              onPress={onOpenGuide}
              pointerEvents="auto"
              style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
            />
          ) : null}
          {loading ? <Centred label="Loading channels" /> : null}
          {tuning && !switching ? (
            <Centred label={snapshot.reconnecting ? "Reconnecting" : "Tuning in"} />
          ) : null}
          {cover ? (
            <PlayerCover
              channelLabel={
                snapshot.channel ? `CH ${snapshot.channel.number} · ${snapshot.channel.name}` : undefined
              }
              density="tv"
              kind={cover.kind}
              {...(cover.message ? { message: cover.message } : {})}
              {...(loadError && onChangeServer
                ? { leaveLabel: "Change server", onLeave: onChangeServer }
                : {})}
              {...(cover.kind === "error"
                ? { onRetry, retryLabel: loadError ? "Try again" : "Retry channel" }
                : {})}
              title={cover.title}
            />
          ) : null}
          {shown && chrome ? (
            <PlayerChrome
              band={chrome.band}
              density="tv"
              formatTime={formatTime}
              keys={{
                density: "tv",
                last: previous
                  ? { number: String(previous.number), onPress: () => onTuneChannel(previous.id) }
                  : undefined,
                moreOpen: layer === "more",
                onMore: () => onLayer(layer === "more" ? undefined : "more"),
                pointer: false,
              }}
              layout="tv"
              live={live}
              menu={
                layer === "audio" ? (
                  // Audio on a television (§9.5): the channel's language, read-only, so its rows take no
                  // focus; it stays on the Audio key, whose OK closes the menu as Menu or Back does.
                  <PlayerMenu density="tv" title="Audio">
                    <PlayerAudio density="tv" options={audioOptions ?? []} />
                  </PlayerMenu>
                ) : layer === "more" ? (
                  // More on a television (§9.5): Channels (Surf), Info, and Start over and Go live while
                  // they apply. Focus starts on its first row; Menu or Back closes it.
                  <PlayerMenu density="tv" heading={false} title="More">
                    <PlayerMore
                      channels={{
                        onPress: () => {
                          onLayer(undefined);
                          onOpenSurf();
                        },
                      }}
                      density="tv"
                      formatTime={formatTime}
                      goLive={
                        live.mode === "live"
                          ? undefined
                          : {
                              detail: formatTime(chrome.window.nowMs),
                              onPress: () => {
                                onLayer(undefined);
                                onGoLive();
                              },
                            }
                      }
                      info={strip ? { onPress: () => onLayer("info") } : undefined}
                      startOver={
                        startOverMs === undefined
                          ? undefined
                          : {
                              detail: chrome.band.title ?? chrome.band.channelName,
                              onPress: () => {
                                onLayer(undefined);
                                onSeek(startOverMs);
                              },
                            }
                      }
                    />
                  </PlayerMenu>
                ) : undefined
              }
              onGoLive={onGoLive}
              schedule={
                strip
                  ? {
                      info: layer === "info",
                      onSelect: onOpenGuide,
                      playheadMs,
                      segments: chrome.segments,
                      window: chrome.window,
                    }
                  : undefined
              }
              utilities={{
                audioOpen: layer === "audio",
                density: "tv",
                onAudio: () => onLayer(layer === "audio" ? undefined : "audio"),
                pointer: false,
              }}
              visible
            />
          ) : null}
          {shown && snapshot.livePlayback?.noticeRevision ? (
            // The paused point fell out of the window and playback went back to live: web says so in a
            // toast; a television says it at the top left, which the J chrome leaves empty there.
            <View
              accessibilityLiveRegion="polite"
              style={{
                backgroundColor: playerChrome.tooltip,
                borderRadius: 8,
                left: EDGE.side,
                paddingHorizontal: 20,
                paddingVertical: 12,
                position: "absolute",
                top: EDGE.top,
                zIndex: 40,
              }}
            >
              <Text density="tv" textRole="reading" tone="primary">
                That paused point is no longer available, so you're back live.
              </Text>
            </View>
          ) : null}
          {typing && numberEntry ? (
            <PlayerReadout
              countdown={countdown}
              density="tv"
              digits={numberEntry.digits}
              name={numberEntry.channelName}
              top={EDGE.top}
            />
          ) : null}
        </ThemeScope>
      ) : null}
    </View>
  );
};

export { WatchingSurface };
