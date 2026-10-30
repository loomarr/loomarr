import { formatClipDuration } from "@loomarr/core/format";
import { type Density, Icon, type IconName, icons, playerChrome, Text } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import { glass, webOnly } from "../glass";
import type { PlayerTransportProps } from "../player-chrome.type";

// The transport (§9.4, plane 4): glass capsules grouped by job — play · live state · volume, then,
// pushed right, view options · window. Play/pause is the one primary control, a white disc; every
// other tool is a quiet glyph (one size, one weight) with a hover well, an amber glyph and dot when
// active, and on a pointer a tooltip naming its key. Tools a later step brings (skip, Start over,
// Subtitles, Info) are not drawn until they work.

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

const Tooltip = ({ density, keyHint, label }: { density: Density; keyHint?: string; label: string }) => (
  <View
    pointerEvents="none"
    style={{
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
    <Text density={density} textRole="guideLabel" tone="primary">
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
  icon: IconName;
  keyHint?: string;
  label: string;
  onPress: () => void;
  pointer: boolean;
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
}: ToolButtonProps) => {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const size = density === "touch" ? 44 : 36;
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
      {pointer && (hovered || focused) ? <Tooltip density={density} keyHint={keyHint} label={label} /> : null}
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
        <Text density={density} shout strong textRole="guideIdent" tone="live">
          Go live
        </Text>
      </Pressable>
    </View>
  );

const PlayerTransport = (props: PlayerTransportProps) => {
  const { density, pointer, playing } = props;
  const touch = density === "touch";
  return (
    <View style={{ alignItems: "center", flexDirection: "row", gap: 10 }}>
      <Capsule>
        <Pressable
          accessibilityLabel={playing ? "Pause" : "Play"}
          accessibilityRole="button"
          onPress={props.onTogglePlay}
          style={{
            alignItems: "center",
            backgroundColor: playerChrome.disc,
            borderRadius: 999,
            height: touch ? 48 : 44,
            justifyContent: "center",
            width: touch ? 48 : 44,
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
      </Capsule>
      <Capsule>
        <LiveState density={density} live={props.live} onGoLive={props.onGoLive} />
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
        />
      </Capsule>
    </View>
  );
};

export { PlayerTransport, ToolButton };
