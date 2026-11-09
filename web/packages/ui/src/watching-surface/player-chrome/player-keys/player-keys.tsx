import {
  brandChroma,
  type Density,
  Icon,
  icons,
  playerChrome,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Pressable, View } from "react-native";

import { raised, webOnly } from "../flat";
import type { PlayerKeysProps } from "../player-chrome.type";

// The keys (§9.3, layer 3): a small remote. Last (hidden until there is a channel to return to), a
// Channel up / down rocker, Play / Pause, Volume (pointer) and More. Neutral solid surfaces with 6 px
// corners on an 8 px grid and room between them; the rocker is not louder than its neighbours. Play is
// the one amber key. Each other key's brand colour is a thin edge on hover and press, nothing more.
// Pointer devices read a short sentence-case legend under each key.

interface KeyProps {
  accessibilityLabel: string;
  children: ReactNode;
  /** The key's brand colour, drawn as its edge while hovered or pressed. */
  edge?: string;
  fill?: string;
  height: number;
  onPress: () => void;
  pressed?: boolean;
  width: number;
}

const Key = ({
  accessibilityLabel,
  children,
  edge = brandChroma[3],
  fill,
  height,
  onPress,
  pressed = false,
  width,
}: KeyProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      accessibilityState={{ expanded: pressed || undefined }}
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={({ pressed: down }) => ({
        alignItems: "center",
        borderRadius: 6,
        height,
        justifyContent: "center",
        width,
        ...raised(),
        ...(fill ? { backgroundColor: fill, borderColor: fill } : null),
        ...(!fill && (hovered || down || pressed) ? { borderColor: edge } : null),
        ...(!fill && (hovered || down) ? { backgroundColor: playerChrome.surfaceHover } : null),
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineOffset: 2,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease, border-color 120ms ease",
        }),
      })}
    >
      {children}
    </Pressable>
  );
};

const Legend = ({ children, density, pointer }: { children: string; density: Density; pointer: boolean }) =>
  pointer ? (
    <Text density={density} strong textRole="guideLabel" tone="primary">
      {children}
    </Text>
  ) : null;

const Column = ({
  children,
  gap,
  marginHorizontal = 0,
}: {
  children: ReactNode;
  gap: number;
  marginHorizontal?: number;
}) => <View style={{ alignItems: "center", gap, marginHorizontal }}>{children}</View>;

const PlayerKeys = ({
  density,
  last,
  moreOpen,
  muted,
  onChannelDown,
  onChannelUp,
  onMore,
  onToggleMute,
  onTogglePlay,
  playing,
  pointer,
  volume,
}: PlayerKeysProps) => {
  const compact = density === "touch";
  const key = pointer ? 64 : compact ? 56 : 64;
  const hero = pointer ? { height: 72, width: 112 } : { height: 64, width: 88 };
  const legendGap = pointer ? 16 : 0;
  const level = muted ? 0 : Math.round(volume * 100);
  return (
    <View style={{ alignItems: "flex-end", flexDirection: "row", flexShrink: 0, gap: pointer ? 24 : 16 }}>
      {last ? (
        <Column gap={legendGap}>
          <Key
            accessibilityLabel={`Last channel, ${last.number}`}
            height={key}
            onPress={last.onPress}
            width={key}
          >
            <View style={{ alignItems: "center", gap: 4 }}>
              <Icon color={playerChrome.text} decorative glyph={icons.swap} size="compact" />
              <Text density={density} strong textRole="guideIdent" tone="primary">
                {last.number}
              </Text>
            </View>
          </Key>
          <Legend density={density} pointer={pointer}>
            Last
          </Legend>
        </Column>
      ) : null}
      {onChannelUp && onChannelDown ? (
        <Column gap={legendGap} marginHorizontal={pointer ? 16 : 8}>
          <View
            style={{
              borderRadius: 6,
              height: key * 2,
              overflow: "hidden",
              width: key,
              ...raised(),
            }}
          >
            <RockerHalf label="Channel up" onPress={onChannelUp}>
              <Icon color={playerChrome.text} decorative glyph={icons.channelUp} size="control" />
            </RockerHalf>
            <View style={{ backgroundColor: playerChrome.hairline, height: 1 }} />
            <RockerHalf label="Channel down" onPress={onChannelDown}>
              <Icon color={playerChrome.text} decorative glyph={icons.channelDown} size="control" />
            </RockerHalf>
          </View>
          <Legend density={density} pointer={pointer}>
            Channel
          </Legend>
        </Column>
      ) : null}
      <Column gap={legendGap}>
        <Key
          accessibilityLabel={playing ? "Pause" : "Play"}
          fill={playerChrome.signal}
          height={hero.height}
          onPress={onTogglePlay}
          width={hero.width}
        >
          <Icon
            color={playerChrome.discInk}
            decorative
            glyph={playing ? icons.pause : icons.play}
            size="control"
          />
        </Key>
        <Legend density={density} pointer={pointer}>
          {playing ? "Pause" : "Play"}
        </Legend>
      </Column>
      {pointer ? (
        <Column gap={legendGap}>
          <Key accessibilityLabel={muted ? "Unmute" : "Mute"} height={key} onPress={onToggleMute} width={key}>
            <Icon
              color={muted ? playerChrome.textMuted : playerChrome.text}
              decorative
              glyph={muted ? icons.volumeMuted : icons.volume}
              size="control"
            />
            {/* The level: a thin white line in the key's foot. */}
            <View
              style={{
                backgroundColor: playerChrome.track,
                bottom: 8,
                height: 4,
                left: 16,
                position: "absolute",
                right: 16,
              }}
            >
              <View style={{ backgroundColor: playerChrome.text, height: 4, width: `${level}%` }} />
            </View>
          </Key>
          <Legend density={density} pointer={pointer}>
            {muted ? "Muted" : "Volume"}
          </Legend>
        </Column>
      ) : null}
      <Column gap={legendGap}>
        <Key
          accessibilityLabel="More"
          edge={brandChroma[4]}
          height={key}
          onPress={onMore}
          pressed={moreOpen}
          width={key}
        >
          <Icon color={playerChrome.text} decorative glyph={icons.more} size="control" />
        </Key>
        <Legend density={density} pointer={pointer}>
          More
        </Legend>
      </Column>
    </View>
  );
};

// One half of the rocker: a flat press target inside the shared frame.
const RockerHalf = ({
  children,
  label,
  onPress,
}: {
  children: ReactNode;
  label: string;
  onPress: () => void;
}) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: hovered ? playerChrome.surfaceHover : "transparent",
        flex: 1,
        justifyContent: "center",
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineOffset: -2,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
      }}
    >
      {children}
    </Pressable>
  );
};

export { PlayerKeys };
