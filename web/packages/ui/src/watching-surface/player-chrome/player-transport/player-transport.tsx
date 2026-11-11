import { formatMmSs } from "@loomarr/core/format";
import {
  type Density,
  Icon,
  type IconName,
  icons,
  playerChrome,
  semanticRadius,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import { nativeFocus, raised, webOnly } from "../flat";
import type { ClipTransportProps } from "../player-chrome.type";

// A clip's transport: flat capsules, Play/pause as the one primary control, a white disc; every other
// tool is a quiet glyph with a hover well and, on a pointer, a tooltip naming its key. The live
// channel's keys are the H remote (player-keys).

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
      ...raised(),
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
      borderColor: playerChrome.hairline,
      borderWidth: 1,
      ...webOnly({ whiteSpace: "nowrap" }),
    }}
  >
    {/* One line: the tooltip hangs off a 36 px button, which would otherwise wrap it word by word. */}
    <Text density={density} numberOfLines={1} textRole="label" tone="primary">
      {label}
    </Text>
    {keyHint ? (
      <View style={{ backgroundColor: playerChrome.surfaceHover, borderRadius: 3, paddingHorizontal: 4 }}>
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
        backgroundColor: active
          ? playerChrome.signalPast
          : hovered
            ? playerChrome.surfaceHover
            : "transparent",
        // Icon-ghost (#2777): a square well like every other icon-ghost button, not a circle.
        borderRadius: semanticRadius.control,
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
        ...nativeFocus(focused),
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
}: Pick<ClipTransportProps, "density" | "muted" | "onVolume" | "volume">) => {
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

export { ClipTransport };
