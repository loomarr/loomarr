import { type Density, playerChrome, Text, ThemeScope } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { ScrollView, View } from "react-native";
import type { PlayerBandData, PlayerTransportProps } from "../player-chrome.type";
import { PlayerFilmstrip, type PlayerFilmstripProps } from "../player-filmstrip";
import { Capsule, LiveState, PlayDisc, StartOver, TextTool, ToolButton } from "../player-transport";

// The portrait phone's panel (§9, mock "portrait"): a portrait picture is too short to carry the
// chrome, so the same planes stack under it instead — the way out and the live state, the lower
// third's identity and progress with the transport under it, what's coming as a filmstrip, then
// surfing, the favourite, Settings and Info. A sheet takes the panel's place while it is open.

interface PlayerPortraitProps {
  band: PlayerBandData;
  density: Density;
  formatTime: (ms: number) => string;
  /** The filmstrip's blocks and window; omitted while the schedule hasn't loaded. */
  filmstrip?: Pick<PlayerFilmstripProps, "onSeek" | "playheadMs" | "segments" | "window">;
  transport: Pick<
    PlayerTransportProps,
    | "infoOpen"
    | "live"
    | "onGoLive"
    | "onInfo"
    | "onLeave"
    | "onSeekBack"
    | "onSeekForward"
    | "onSettings"
    | "onStartOver"
    | "onTogglePlay"
    | "playing"
    | "settingsOpen"
  >;
  /** Channel up/down, Previous and the favourite; omitted where Watch has no tuner. */
  surf?: {
    canPrevious: boolean;
    canSurf: boolean;
    favourite: boolean;
    onChannelDown: () => void;
    onChannelUp: () => void;
    onFavourite?: () => void;
    onPrevious: () => void;
  };
  /** The open sheet, drawn inline in the panel's place. */
  sheet?: ReactNode;
}

const PlayerPortrait = ({
  band,
  density,
  filmstrip,
  formatTime,
  sheet,
  surf,
  transport,
}: PlayerPortraitProps) => (
  <ThemeScope theme="dark">
    {sheet ?? (
      <ScrollView
        contentContainerStyle={{
          flexGrow: 1,
          gap: 16,
          paddingBottom: 20,
          paddingHorizontal: 16,
          paddingTop: 12,
        }}
        style={{ backgroundColor: playerChrome.sheet, flex: 1 }}
      >
        <View style={{ alignItems: "center", flexDirection: "row", gap: 8, justifyContent: "space-between" }}>
          <Capsule>
            <ToolButton
              density={density}
              icon="close"
              label="Leave Watch"
              onPress={transport.onLeave}
              pointer={false}
            />
          </Capsule>
          <Capsule>
            <LiveState density={density} live={transport.live} onGoLive={transport.onGoLive} />
          </Capsule>
        </View>

        <View>
          <Text
            density={density}
            numberOfLines={1}
            shout
            textRole="guideMeta"
            tone="secondary"
            tracking={0.14}
          >
            {`CH ${band.channelNumber} · ${band.channelName}`}
          </Text>
          {band.programmeLine ? (
            <Text
              density={density}
              numberOfLines={2}
              strong
              style={{ marginTop: 4 }}
              textRole="cardTitle"
              tone="primary"
            >
              {band.programmeLine}
            </Text>
          ) : null}
          {band.progress !== undefined ? (
            <View
              style={{
                backgroundColor: playerChrome.track,
                borderRadius: 2,
                height: 4,
                marginTop: 10,
                overflow: "hidden",
              }}
            >
              <View
                style={{
                  backgroundColor: band.inBreak ? playerChrome.breakAccent : playerChrome.signal,
                  height: "100%",
                  width: `${Math.round(band.progress * 100)}%`,
                }}
              />
            </View>
          ) : null}
          {band.timeLabel ? (
            <Text density={density} style={{ marginTop: 6 }} textRole="cardTime" tone="secondary">
              {band.timeLabel}
            </Text>
          ) : null}
          <View style={{ alignItems: "center", flexDirection: "row", gap: 10, marginTop: 12 }}>
            <Capsule>
              {transport.onSeekBack ? (
                <ToolButton
                  density={density}
                  icon="seekBack"
                  label="Back 10 seconds"
                  onPress={transport.onSeekBack}
                  pointer={false}
                />
              ) : null}
              <PlayDisc density={density} onPress={transport.onTogglePlay} playing={transport.playing} />
              {transport.onSeekForward ? (
                <ToolButton
                  density={density}
                  disabled={transport.live.mode === "live"}
                  icon="seekForward"
                  label="Forward 10 seconds"
                  onPress={transport.onSeekForward}
                  pointer={false}
                />
              ) : null}
            </Capsule>
            <View style={{ flex: 1 }} />
            {transport.onStartOver ? (
              <Capsule>
                <StartOver density={density} onPress={transport.onStartOver} />
              </Capsule>
            ) : null}
          </View>
        </View>

        {filmstrip ? (
          <View style={{ borderTopColor: playerChrome.glassRing, borderTopWidth: 1, gap: 8, paddingTop: 12 }}>
            <Text density={density} shout textRole="guideMeta" tone="info" tracking={0.16}>
              Coming up
            </Text>
            <PlayerFilmstrip density={density} formatTime={formatTime} {...filmstrip} />
          </View>
        ) : null}

        {/* Five 44 px targets and Previous only just fit a 390 px phone: a narrower one wraps the
            Settings capsule to its own line rather than cutting it off. */}
        <View
          style={{ alignItems: "center", flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: "auto" }}
        >
          {surf ? (
            <>
              <Capsule>
                <ToolButton
                  density={density}
                  disabled={!surf.canSurf}
                  icon="channelUp"
                  label="Channel up"
                  onPress={surf.onChannelUp}
                  pointer={false}
                />
                <ToolButton
                  density={density}
                  disabled={!surf.canSurf}
                  icon="channelDown"
                  label="Channel down"
                  onPress={surf.onChannelDown}
                  pointer={false}
                />
                <TextTool
                  density={density}
                  disabled={!surf.canPrevious}
                  label="Previous"
                  onPress={surf.onPrevious}
                />
              </Capsule>
              {surf.onFavourite ? (
                <Capsule>
                  <ToolButton
                    active={surf.favourite}
                    density={density}
                    filled
                    icon="favourite"
                    label={surf.favourite ? "Remove from Favorites" : "Add to Favorites"}
                    onPress={surf.onFavourite}
                    pointer={false}
                  />
                </Capsule>
              ) : null}
            </>
          ) : null}
          {/* Pushed right by its margin, not a spacer: a spacer is one more gap to fit. */}
          <View style={{ marginLeft: "auto" }}>
            <Capsule>
              {transport.onSettings ? (
                <ToolButton
                  active={transport.settingsOpen}
                  density={density}
                  icon="settings"
                  label="Settings"
                  onPress={transport.onSettings}
                  pointer={false}
                />
              ) : null}
              {transport.onInfo ? (
                <ToolButton
                  active={transport.infoOpen}
                  density={density}
                  icon="info"
                  label="Info"
                  onPress={transport.onInfo}
                  pointer={false}
                />
              ) : null}
            </Capsule>
          </View>
        </View>
      </ScrollView>
    )}
  </ThemeScope>
);

export type { PlayerPortraitProps };
export { PlayerPortrait };
