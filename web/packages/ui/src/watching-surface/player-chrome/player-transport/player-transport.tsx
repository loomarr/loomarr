import { formatClipDuration } from "@loomarr/core/format";
import { type Density, Icon, type IconName, icons, playerChrome, Text } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import { focusRing, glass, webOnly } from "../glass";
import type { PlayerTransportProps } from "../player-chrome.type";

// The transport (§9.4, plane 4): glass capsules grouped by job — ⟲ play ⟳ · live state and Start
// over · volume, then, pushed right, view options · window. Play/pause is the one primary control, a
// white disc; every other tool is a quiet glyph (one size, one weight) with a hover well, an amber
// glyph and dot when active, and on a pointer a tooltip naming its key. A tool the caller leaves out
// (the player cannot seek; Info with no programme; Subtitles with no track) is not drawn.

const Capsule = ({ children, end }: { children: ReactNode; end?: boolean }) => (
  <View
    style={{
      alignItems: "center",
      borderRadius: 999,
      flexDirection: "row",
      flexShrink: 0,
      gap: 2,
      padding: 4,
      paddingRight: end ? 12 : 4,
      ...glass(),
    }}
  >
    {children}
  </View>
);

// Which way a tooltip hangs: centred over its tool, or held inside the screen at the bar's two ends.
type TipAlign = "center" | "start" | "end";

const tipAnchor = (align: TipAlign) =>
  align === "start"
    ? { left: 0 }
    : align === "end"
      ? { right: 0 }
      : { left: "50%" as const, transform: [{ translateX: "-50%" }] };

const Tooltip = ({
  align,
  density,
  keyHint,
  label,
}: {
  align: TipAlign;
  density: Density;
  keyHint?: string;
  label: string;
}) => (
  <View
    pointerEvents="none"
    style={{
      ...tipAnchor(align),
      alignItems: "center",
      backgroundColor: playerChrome.tooltip,
      borderRadius: 6,
      bottom: "100%",
      flexDirection: "row",
      gap: 6,
      marginBottom: 10,
      paddingHorizontal: 8,
      paddingVertical: 4,
      position: "absolute",
      ...webOnly({ boxShadow: `0 0 0 1px ${playerChrome.glassRing}`, whiteSpace: "nowrap" }),
    }}
  >
    {/* One line: the tooltip hangs off a 36 px button, which would otherwise wrap it word by word. */}
    <Text density={density} numberOfLines={1} textRole="guideLabel" tone="primary">
      {label}
    </Text>
    {keyHint ? (
      <View style={{ backgroundColor: playerChrome.glassTop, borderRadius: 3, paddingHorizontal: 4 }}>
        <Text density={density} textRole="guideMeta" tone="secondary">
          {keyHint}
        </Text>
      </View>
    ) : null}
  </View>
);

interface ToolButtonProps {
  active?: boolean;
  density: Density;
  disabled?: boolean;
  /** The glyph drawn solid while active (a chosen favourite's star). */
  filled?: boolean;
  icon: IconName;
  keyHint?: string;
  label: string;
  onPress: () => void;
  pointer: boolean;
  tipAlign?: TipAlign;
}

const ToolButton = ({
  active = false,
  density,
  disabled = false,
  filled = false,
  icon,
  keyHint,
  label,
  onPress,
  pointer,
  tipAlign = "center",
}: ToolButtonProps) => {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const size = density === "touch" ? 44 : 36;
  // On a TV, focus stands in for hover (§9): the well lights and the tool names itself.
  const tv = density === "tv";
  const lit = hovered || (tv && focused);
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      accessibilityState={{ disabled, expanded: active || undefined }}
      disabled={disabled}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: active ? playerChrome.signalPast : lit ? playerChrome.glassTop : "transparent",
        borderRadius: 999,
        height: size,
        justifyContent: "center",
        opacity: disabled ? 0.3 : 1,
        width: size,
        ...focusRing(focused, density),
        ...webOnly({ transition: "background-color 140ms ease" }),
      }}
    >
      <Icon
        color={active ? playerChrome.signal : lit ? playerChrome.text : playerChrome.textSecondary}
        decorative
        filled={filled && active}
        glyph={icons[icon]}
        size="player"
      />
      {active ? (
        <View
          style={{
            backgroundColor: playerChrome.signal,
            borderRadius: 2,
            bottom: 4,
            height: 4,
            position: "absolute",
            width: 4,
          }}
        />
      ) : null}
      {(pointer && (hovered || focused)) || (tv && focused) ? (
        <Tooltip align={tipAlign} density={density} keyHint={pointer ? keyHint : undefined} label={label} />
      ) : null}
    </Pressable>
  );
};

const VolumeSlider = ({
  density,
  muted,
  onVolume,
  volume,
}: Pick<PlayerTransportProps, "density" | "muted" | "onVolume" | "volume">) => {
  const [width, setWidth] = useState(0);
  const level = muted ? 0 : volume;
  const step = (delta: number) => onVolume(Math.min(1, Math.max(0, volume + delta)));
  return (
    <Pressable
      accessibilityLabel="Volume"
      accessibilityRole="adjustable"
      // The aria-value props, not accessibilityValue: React Native for Web drops the latter, which left
      // the web's slider with no value at all (axe: aria-required-attr).
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={Math.round(level * 100)}
      accessibilityActions={[{ name: "increment" }, { name: "decrement" }]}
      onAccessibilityAction={(e) => step(e.nativeEvent.actionName === "increment" ? 0.1 : -0.1)}
      onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)}
      onPress={(e) => width > 0 && onVolume(Math.min(1, Math.max(0, e.nativeEvent.locationX / width)))}
      style={{ height: density === "touch" ? 44 : 36, justifyContent: "center", width: 80 }}
      // The web has no accessibility actions: the arrow keys step the level, as a slider's do.
      {...(Platform.OS === "web" && {
        onKeyDown: (e: unknown) => {
          const key = e as { key: string; preventDefault: () => void };
          if (key.key === "ArrowRight" || key.key === "ArrowUp") step(0.1);
          else if (key.key === "ArrowLeft" || key.key === "ArrowDown") step(-0.1);
          else return;
          key.preventDefault();
        },
      })}
    >
      <View style={{ backgroundColor: playerChrome.track, borderRadius: 2, height: 4 }}>
        <View
          style={{
            backgroundColor: playerChrome.text,
            borderRadius: 2,
            height: "100%",
            width: `${Math.round(level * 100)}%`,
          }}
        />
      </View>
      <View
        style={{
          backgroundColor: playerChrome.disc,
          borderRadius: 999,
          height: 10,
          left: `${Math.round(level * 100)}%`,
          marginLeft: -5,
          position: "absolute",
          width: 10,
        }}
      />
    </Pressable>
  );
};

const LiveState = ({
  density,
  live,
  onGoLive,
}: Pick<PlayerTransportProps, "density" | "live" | "onGoLive">) => {
  const [focused, setFocused] = useState(false);
  return live.mode === "live" ? (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 6, height: 36, paddingHorizontal: 12 }}>
      <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 7, width: 7 }} />
      <Text density={density} shout strong textRole="guideIdent" tone="live">
        Live
      </Text>
    </View>
  ) : (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 10, paddingLeft: 12 }}>
      <Text accessibilityLiveRegion="polite" density={density} shout textRole="guideMeta" tone="secondary">
        {`${live.mode === "paused" ? "Paused · " : ""}${formatClipDuration(live.lagSeconds * 1000)} behind`}
      </Text>
      <Pressable
        accessibilityRole="button"
        onBlur={() => setFocused(false)}
        onFocus={() => setFocused(true)}
        onPress={onGoLive}
        style={{
          alignItems: "center",
          borderColor: playerChrome.live,
          borderRadius: 999,
          borderWidth: 1,
          flexDirection: "row",
          gap: 6,
          height: density === "touch" ? 40 : 30,
          paddingHorizontal: 10,
          // The pill keeps its own red edge until focus takes it.
          ...(focused ? focusRing(true, density) : {}),
        }}
      >
        <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 7, width: 7 }} />
        <Text density={density} shout strong textRole="guideIdent" tone="live">
          Go live
        </Text>
      </Pressable>
    </View>
  );
};

// A quiet text tool in a capsule: Start over beside the live state (§9.4), back to the programme's
// start, and the portrait panel's Previous.
const TextTool = ({
  density,
  disabled = false,
  icon,
  label,
  onPress,
}: {
  density: Density;
  disabled?: boolean;
  icon?: IconName;
  label: string;
  onPress: () => void;
}) => {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const lit = hovered || (density === "tv" && focused);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled }}
      disabled={disabled}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: lit ? playerChrome.glassTop : "transparent",
        borderRadius: 999,
        flexDirection: "row",
        gap: 6,
        height: density === "touch" ? 40 : 32,
        opacity: disabled ? 0.3 : 1,
        paddingHorizontal: 12,
        ...focusRing(focused, density),
        ...webOnly({ transition: "background-color 140ms ease" }),
      }}
    >
      {icon ? <Icon color={playerChrome.text} decorative glyph={icons[icon]} size="compact" /> : null}
      <Text density={density} textRole="guideLabel" tone="primary">
        {label}
      </Text>
    </Pressable>
  );
};

const StartOver = ({ density, onPress }: { density: Density; onPress: () => void }) => (
  <TextTool density={density} icon="startOver" label="Start over" onPress={onPress} />
);

// Play/pause: the transport's one primary control, a white disc.
const PlayDisc = ({
  density,
  onPress,
  playing,
}: {
  density: Density;
  onPress: () => void;
  playing: boolean;
}) => {
  const size = density === "touch" ? 48 : 44;
  const [focused, setFocused] = useState(false);
  return (
    <Pressable
      accessibilityLabel={playing ? "Pause" : "Play"}
      accessibilityRole="button"
      // A remote's OK brings the controls up on Play/Pause (#2088 decision 15).
      hasTVPreferredFocus={density === "tv"}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: playerChrome.disc,
        borderRadius: 999,
        height: size,
        justifyContent: "center",
        width: size,
        ...focusRing(focused, density),
        ...webOnly({
          boxShadow: "inset 0 -2px 0 rgba(0, 0, 0, 0.12), 0 6px 18px -6px rgba(255, 255, 255, 0.35)",
        }),
      }}
    >
      <Icon
        color={playerChrome.discInk}
        decorative
        glyph={playing ? icons.pause : icons.play}
        size="default"
      />
    </Pressable>
  );
};

const PlayerTransport = (props: PlayerTransportProps) => {
  const { density, pointer, playing } = props;
  const window = Boolean(props.onLeave || (pointer && (props.onPictureInPicture || props.onFullscreen)));
  return (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 10 }}>
      <Capsule>
        {props.onSeekBack ? (
          <ToolButton
            density={density}
            icon="seekBack"
            keyHint="←"
            label="Back 10 seconds"
            onPress={props.onSeekBack}
            pointer={pointer}
            tipAlign="start"
          />
        ) : null}
        <PlayDisc density={density} onPress={props.onTogglePlay} playing={playing} />
        {props.onSeekForward ? (
          <ToolButton
            density={density}
            disabled={props.live.mode === "live"}
            icon="seekForward"
            keyHint="→"
            label="Forward 10 seconds"
            onPress={props.onSeekForward}
            pointer={pointer}
          />
        ) : null}
      </Capsule>
      <Capsule>
        <LiveState density={density} live={props.live} onGoLive={props.onGoLive} />
        {props.onStartOver ? <StartOver density={density} onPress={props.onStartOver} /> : null}
      </Capsule>
      {pointer ? (
        <Capsule end>
          <ToolButton
            density={density}
            icon={props.muted ? "volumeMuted" : "volume"}
            keyHint="M"
            label={props.muted ? "Unmute" : "Mute"}
            onPress={props.onToggleMute}
            pointer={pointer}
          />
          <VolumeSlider
            density={density}
            muted={props.muted}
            onVolume={props.onVolume}
            volume={props.volume}
          />
        </Capsule>
      ) : null}
      <View style={{ flex: 1 }} />
      <Capsule>
        {props.onSubtitles ? (
          <ToolButton
            active={props.subtitlesOn}
            density={density}
            icon="subtitles"
            keyHint="C"
            label={props.subtitlesOn ? "Next subtitles" : "Subtitles"}
            onPress={props.onSubtitles}
            pointer={pointer}
          />
        ) : null}
        {props.onSettings ? (
          <ToolButton
            active={props.settingsOpen}
            density={density}
            icon="settings"
            label="Settings"
            onPress={props.onSettings}
            pointer={pointer}
          />
        ) : null}
        {props.onInfo ? (
          <ToolButton
            active={props.infoOpen}
            density={density}
            icon="info"
            keyHint="I"
            label="Info"
            onPress={props.onInfo}
            pointer={pointer}
          />
        ) : null}
        {props.onGuide ? (
          <ToolButton
            density={density}
            icon="guide"
            label="Guide"
            onPress={props.onGuide}
            pointer={pointer}
            tipAlign={props.onChannels || window ? "center" : "end"}
          />
        ) : null}
        {props.onChannels ? (
          <ToolButton
            active={props.channelsOpen}
            density={density}
            icon="list"
            keyHint="G"
            label="Channels"
            onPress={props.onChannels}
            pointer={pointer}
            tipAlign={window ? "center" : "end"}
          />
        ) : null}
      </Capsule>
      {/* A touch window has only the way out (the mock's landscape): the phone's own browser owns
          fullscreen and picture-in-picture there. A TV has none of the three. */}
      {window ? (
        <Capsule>
          {pointer && props.onPictureInPicture ? (
            <ToolButton
              active={props.pictureInPicture}
              density={density}
              icon="pictureInPicture"
              keyHint="P"
              label={props.pictureInPicture ? "Exit picture-in-picture" : "Picture-in-picture"}
              onPress={props.onPictureInPicture}
              pointer={pointer}
            />
          ) : null}
          {pointer && props.onFullscreen ? (
            <ToolButton
              density={density}
              icon={props.fullscreen ? "fullscreenExit" : "fullscreen"}
              keyHint="F"
              label={props.fullscreen ? "Exit fullscreen" : "Fullscreen"}
              onPress={props.onFullscreen}
              pointer={pointer}
            />
          ) : null}
          {props.onLeave ? (
            <ToolButton
              density={density}
              icon="close"
              label="Leave Watch"
              onPress={props.onLeave}
              pointer={pointer}
              tipAlign="end"
            />
          ) : null}
        </Capsule>
      ) : null}
    </View>
  );
};

export { Capsule, LiveState, PlayDisc, PlayerTransport, StartOver, TextTool, ToolButton };
