import { formatClipDuration, formatMmSs } from "@loomarr/core/format";
import {
  type Density,
  Icon,
  type IconName,
  icons,
  playerChrome,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import { glass, webOnly } from "../glass";
import type { ClipTransportProps, PlayerTransportProps } from "../player-chrome.type";

// The transport (§9.4, plane 4): glass capsules grouped by job — ⟲ play ⟳ · live state and Start
// over · volume, then, pushed right, view options · window. Play/pause is the one primary control, a
// white disc; every other tool is a quiet glyph (one size, one weight) with a hover well, an amber
// glyph and dot when active, and on a pointer a tooltip naming its key. A tool the caller leaves out
// (the player cannot seek; Info with no programme; Subtitles until its step) is not drawn.

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
    <Text density={density} numberOfLines={1} textRole="label" tone="primary">
      {label}
    </Text>
    {keyHint ? (
      <View style={{ backgroundColor: playerChrome.glassTop, borderRadius: 3, paddingHorizontal: 4 }}>
        <Text density={density} textRole="mono" tone="secondary">
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
  icon,
  keyHint,
  label,
  onPress,
  pointer,
  tipAlign = "center",
}: ToolButtonProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  const size = density === "touch" ? 44 : 36;
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      accessibilityState={{ disabled, expanded: active || undefined }}
      disabled={disabled}
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: active ? playerChrome.signalPast : hovered ? playerChrome.glassTop : "transparent",
        borderRadius: 999,
        height: size,
        justifyContent: "center",
        opacity: disabled ? 0.3 : 1,
        width: size,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 140ms ease",
        }),
      }}
    >
      <Icon
        color={active ? playerChrome.signal : hovered ? playerChrome.text : playerChrome.textSecondary}
        decorative
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
      {pointer && (hovered || focused) ? (
        <Tooltip align={tipAlign} density={density} keyHint={keyHint} label={label} />
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
}: Pick<PlayerTransportProps, "density" | "live" | "onGoLive">) =>
  live.mode === "live" ? (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 6, height: 36, paddingHorizontal: 12 }}>
      <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 7, width: 7 }} />
      <Text density={density} shout strong textRole="label" tone="live">
        Live
      </Text>
    </View>
  ) : (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 10, paddingLeft: 12 }}>
      <Text accessibilityLiveRegion="polite" density={density} shout textRole="mono" tone="secondary">
        {`${live.mode === "paused" ? "Paused · " : ""}${formatClipDuration(live.lagSeconds * 1000)} behind`}
      </Text>
      <Pressable
        accessibilityRole="button"
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
        }}
      >
        <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 7, width: 7 }} />
        <Text density={density} shout strong textRole="label" tone="live">
          Go live
        </Text>
      </Pressable>
    </View>
  );

// Start over (§9.4): a quiet text tool beside the live state, back to the programme's start.
const StartOver = ({ density, onPress }: { density: Density; onPress: () => void }) => {
  const [hovered, setHovered] = useState(false);
  return (
    <Pressable
      accessibilityRole="button"
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: hovered ? playerChrome.glassTop : "transparent",
        borderRadius: 999,
        flexDirection: "row",
        gap: 6,
        height: density === "touch" ? 40 : 32,
        paddingHorizontal: 12,
        ...webOnly({ transition: "background-color 140ms ease" }),
      }}
    >
      <Icon color={playerChrome.text} decorative glyph={icons.startOver} size="compact" />
      <Text density={density} textRole="label" tone="primary">
        Start over
      </Text>
    </Pressable>
  );
};

// Play/pause: the one primary control, a white disc. Its name follows the state, so a screen reader
// never hears "Play" over a playing picture.
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
  return (
    <Pressable
      accessibilityLabel={playing ? "Pause" : "Play"}
      accessibilityRole="button"
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: playerChrome.disc,
        borderRadius: 999,
        height: size,
        justifyContent: "center",
        width: size,
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
        <ToolButton
          active={props.settingsOpen}
          density={density}
          icon="settings"
          label="Settings"
          onPress={props.onSettings}
          pointer={pointer}
        />
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
        {props.onChannels ? (
          <ToolButton
            active={props.channelsOpen}
            density={density}
            icon="list"
            keyHint="G"
            label="Channels"
            onPress={props.onChannels}
            pointer={pointer}
          />
        ) : null}
      </Capsule>
      <Capsule>
        {props.onFullscreen ? (
          <ToolButton
            density={density}
            icon={props.fullscreen ? "fullscreenExit" : "fullscreen"}
            label={props.fullscreen ? "Exit fullscreen" : "Fullscreen"}
            onPress={props.onFullscreen}
            pointer={pointer}
          />
        ) : null}
        <ToolButton
          density={density}
          icon="close"
          label="Leave Watch"
          onPress={props.onLeave}
          pointer={pointer}
          tipAlign="end"
        />
      </Capsule>
    </View>
  );
};

// A clip's transport: the live transport's own pieces, minus everything a finite file has no use
// for (no window to seek through, no live state, no sheets). Play · volume · elapsed, then,
// pushed right, fullscreen.
const ClipTransport = ({
  density,
  durationMs,
  elapsedMs,
  fullscreen,
  muted,
  onFullscreen,
  onToggleMute,
  onTogglePlay,
  onVolume,
  playing,
  pointer,
  volume,
}: ClipTransportProps) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 10 }}>
    <Capsule>
      <PlayDisc density={density} onPress={onTogglePlay} playing={playing} />
    </Capsule>
    <Capsule end>
      <ToolButton
        density={density}
        icon={muted ? "volumeMuted" : "volume"}
        keyHint="M"
        label={muted ? "Unmute" : "Mute"}
        onPress={onToggleMute}
        pointer={pointer}
      />
      <VolumeSlider density={density} muted={muted} onVolume={onVolume} volume={volume} />
    </Capsule>
    {durationMs !== undefined ? (
      <Text density={density} textRole="mono" tone="secondary">
        {formatMmSs(elapsedMs)} / {formatMmSs(durationMs)}
      </Text>
    ) : null}
    <View style={{ flex: 1 }} />
    <Capsule>
      <ToolButton
        density={density}
        icon={fullscreen ? "fullscreenExit" : "fullscreen"}
        label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
        onPress={onFullscreen}
        pointer={pointer}
        tipAlign="end"
      />
    </Capsule>
  </View>
);

export { ClipTransport, PlayerTransport, ToolButton };
