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
import { Pressable, View } from "react-native";

import { nativeFocus, raised, webOnly } from "../flat";
import type { PlayerKeysProps, PlayerUtilitiesProps } from "../player-chrome.type";

// The keys (§9.3, variant J): a small remote. Last channel (only when there is one to return to,
// then a hairline), Channel down, Play / Pause, Channel up, Volume (desktop) and More. Solid squares
// with a hairline and soft corners; Play is the one filled key and it is white, so amber stays on the
// channel block and focus. A key's legend shows under it only while it is pointed at or focused.
// A television has four keys: Last channel, Subtitles, Audio and More. Its remote has the rest.

interface KeyProps {
  accessibilityLabel: string;
  children: ReactNode;
  height: number;
  /** The short word under the key, while it is pointed at or focused. */
  legend?: string | undefined;
  /** Play: the one filled key, white with an ink glyph. */
  ok?: boolean;
  onPress: () => void;
  /** What the key opens is open: a tone lighter, and it says it is expanded. Left out on a key that opens nothing. */
  pressed?: boolean | undefined;
  /** Legends, for a pointer or a remote's focus; a phone has neither. */
  legends: boolean;
  width: number;
}

const Key = ({
  accessibilityLabel,
  children,
  height,
  legend,
  legends,
  ok = false,
  onPress,
  pressed,
  width,
}: KeyProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <View style={{ alignItems: "center", gap: 8 }}>
      <Pressable
        accessibilityLabel={accessibilityLabel}
        accessibilityRole="button"
        // aria-expanded, not accessibilityState: React Native for Web 0.21 drops the latter. A key that
        // opens nothing (pressed undefined) says nothing about it.
        aria-expanded={pressed}
        {...focusHandlers}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        onPress={onPress}
        style={({ pressed: down }) => ({
          alignItems: "center",
          borderRadius: 12,
          height,
          justifyContent: "center",
          width,
          ...(ok
            ? {
                backgroundColor: playerChrome.disc,
                borderColor: playerChrome.disc,
                borderWidth: 1,
                opacity: hovered || down ? 0.9 : 1,
              }
            : raised()),
          ...(!ok && (hovered || down) ? { backgroundColor: playerChrome.surfaceHover } : null),
          ...(!ok && pressed
            ? { backgroundColor: playerChrome.surfaceHover, borderColor: playerChrome.textSecondary }
            : null),
          ...webOnly({
            outlineColor: playerChrome.signal,
            outlineOffset: 3,
            outlineStyle: focused ? "solid" : "none",
            outlineWidth: 2,
            transition: "background-color 120ms ease, border-color 120ms ease, opacity 120ms ease",
          }),
          ...nativeFocus(focused),
        })}
      >
        {children}
      </Pressable>
      {legends && legend ? (
        // The legend keeps its room while hidden, so a key showing it moves nothing.
        <View
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants"
          pointerEvents="none"
          style={{ opacity: hovered || focused ? 1 : 0, ...webOnly({ transition: "opacity 120ms ease" }) }}
        >
          <Text density="pointer" numberOfLines={1} textRole="cardMeta" tone="secondary">
            {legend}
          </Text>
        </View>
      ) : null}
    </View>
  );
};

const Glyph = ({ density, ink = false, name }: { density: Density; ink?: boolean; name: IconName }) => (
  <Icon
    color={ink ? playerChrome.discInk : playerChrome.text}
    decorative
    glyph={icons[name]}
    size={density === "touch" ? "control" : "touch"}
  />
);

/** Last channel: a swap glyph over the number it returns to. */
const LastFace = ({ number, pointer }: { number: string; pointer: boolean }) => (
  <View style={{ alignItems: "center", gap: 2 }}>
    <Icon color={playerChrome.text} decorative glyph={icons.swap} size={pointer ? "player" : "compact"} />
    <Text density={pointer ? "pointer" : "touch"} strong textRole="mono" tone="secondary">
      {number}
    </Text>
  </View>
);

const PlayerKeys = ({
  audio,
  density,
  last,
  moreOpen,
  onChannelDown,
  onChannelUp,
  onMore,
  onTogglePlay,
  playing,
  pointer,
  volume,
}: PlayerKeysProps & {
  /** A television's Subtitles and Audio, which are main keys there (§9.3). */
  audio?: PlayerUtilitiesProps | undefined;
}) => {
  const tv = density === "tv";
  const legends = pointer || tv;
  const size = pointer || tv ? 56 : 48;
  const okWidth = pointer ? 80 : 64;
  const lastKey = last ? (
    <Key
      accessibilityLabel={`Last channel, ${last.number}`}
      height={size}
      legend="Last channel"
      legends={legends}
      onPress={last.onPress}
      width={size}
    >
      <LastFace number={last.number} pointer={pointer || tv} />
    </Key>
  ) : null;
  const moreKey = onMore ? (
    <Key
      accessibilityLabel="More"
      height={size}
      legend="More"
      legends={legends}
      onPress={onMore}
      pressed={moreOpen}
      width={size}
    >
      <Glyph density={density} name="more" />
    </Key>
  ) : null;

  if (tv)
    return (
      <View style={{ alignItems: "flex-start", flexDirection: "row", flexShrink: 0, gap: 16 }}>
        {lastKey}
        {audio?.onSubtitles ? (
          <Key
            accessibilityLabel="Subtitles"
            height={size}
            legend="Subtitles"
            legends
            onPress={audio.onSubtitles}
            pressed={audio.subtitlesOpen}
            width={size}
          >
            <Glyph density={density} name="captions" />
          </Key>
        ) : null}
        {audio?.onAudio ? (
          <Key
            accessibilityLabel="Audio"
            height={size}
            legend="Audio"
            legends
            onPress={audio.onAudio}
            pressed={audio.audioOpen}
            width={size}
          >
            <Glyph density={density} name="audio" />
          </Key>
        ) : null}
        {moreKey}
      </View>
    );

  return (
    <View
      style={{
        alignItems: "flex-start",
        flexDirection: "row",
        flexShrink: 0,
        gap: pointer ? 16 : 12,
      }}
    >
      {lastKey ? (
        <>
          {lastKey}
          <View
            style={{ backgroundColor: playerChrome.hairline, height: size, marginHorizontal: 4, width: 1 }}
          />
        </>
      ) : null}
      {onChannelDown ? (
        <Key
          accessibilityLabel="Channel down"
          height={size}
          legend="Channel down"
          legends={legends}
          onPress={onChannelDown}
          width={size}
        >
          <Glyph density={density} name="channelDown" />
        </Key>
      ) : null}
      {onTogglePlay ? (
        <Key
          accessibilityLabel={playing ? "Pause" : "Play"}
          height={size}
          legend={playing ? "Pause" : "Play"}
          legends={legends}
          ok
          onPress={onTogglePlay}
          width={okWidth}
        >
          <Glyph density={density} ink name={playing ? "pause" : "play"} />
        </Key>
      ) : null}
      {onChannelUp ? (
        <Key
          accessibilityLabel="Channel up"
          height={size}
          legend="Channel up"
          legends={legends}
          onPress={onChannelUp}
          width={size}
        >
          <Glyph density={density} name="channelUp" />
        </Key>
      ) : null}
      {volume ? (
        // Volume opens the level in the menu's place (§9.5); M mutes.
        <Key
          accessibilityLabel="Volume"
          height={size}
          legend={volume.muted ? "Muted" : "Volume"}
          legends={legends}
          onPress={volume.onPress}
          pressed={volume.open}
          width={size}
        >
          <Glyph density={density} name={volume.muted ? "volumeMuted" : "volume"} />
        </Key>
      ) : null}
      {moreKey}
    </View>
  );
};

export { PlayerKeys };
