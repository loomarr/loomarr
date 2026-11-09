import {
  brandChroma,
  Icon,
  type IconName,
  icons,
  playerChrome,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

import { raised, webOnly } from "../flat";
import type { PlayerTopRowProps } from "../player-chrome.type";

// The top row (§9.3), upper right: Subtitles and Audio, each a glyph and its word so neither can be
// mistaken for the other, then Fullscreen (pointer) and Leave. The same solid key language as the
// keys. Subtitles on is a small white dot, never amber. Pointer tooltips name the shortcut.

const SIZE = 48;

interface TopKeyProps {
  active?: boolean;
  /** The key that does this on a keyboard, named in the tooltip. */
  hint?: string;
  icon: IconName;
  label: string;
  onPress: () => void;
  pointer: boolean;
  pressed?: boolean;
  /** The word beside the glyph; absent, an icon-only key. */
  text?: string;
  /** The key's thin edge on hover and press. */
  edge?: string;
}

const TopKey = ({
  active,
  edge = brandChroma[3],
  hint,
  icon,
  label,
  onPress,
  pointer,
  pressed,
  text,
}: TopKeyProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <View>
      <Pressable
        accessibilityLabel={label}
        accessibilityRole="button"
        accessibilityState={{ expanded: pressed || undefined }}
        {...focusHandlers}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        onPress={onPress}
        style={({ pressed: down }) => ({
          alignItems: "center",
          borderRadius: 6,
          flexDirection: "row",
          gap: 8,
          height: SIZE,
          justifyContent: "center",
          paddingLeft: text ? 12 : 0,
          paddingRight: text ? 16 : 0,
          width: text ? undefined : SIZE,
          ...raised(),
          ...(hovered || down || pressed
            ? { backgroundColor: playerChrome.surfaceHover, borderColor: edge }
            : null),
          ...webOnly({
            outlineColor: playerChrome.signal,
            outlineOffset: 2,
            outlineStyle: focused ? "solid" : "none",
            outlineWidth: 2,
            transition: "background-color 120ms ease, border-color 120ms ease",
          }),
        })}
      >
        <Icon color={playerChrome.text} decorative glyph={icons[icon]} size="control" />
        {text ? (
          <Text density="touch" strong textRole="label" tone="primary">
            {text}
          </Text>
        ) : null}
        {active ? (
          <View
            style={{
              backgroundColor: playerChrome.text,
              borderRadius: 4,
              height: 8,
              position: "absolute",
              right: 6,
              top: 6,
              width: 8,
            }}
          />
        ) : null}
      </Pressable>
      {pointer && hint && (hovered || focused) ? (
        <View
          pointerEvents="none"
          style={{
            alignItems: "center",
            backgroundColor: playerChrome.tooltip,
            borderColor: playerChrome.hairline,
            borderRadius: 6,
            borderWidth: 1,
            flexDirection: "row",
            gap: 8,
            paddingHorizontal: 8,
            paddingVertical: 4,
            position: "absolute",
            right: 0,
            top: SIZE + 8,
            ...webOnly({ whiteSpace: "nowrap" }),
          }}
        >
          <Text density="pointer" numberOfLines={1} textRole="guideLabel" tone="primary">
            {label}
          </Text>
          <Text density="pointer" textRole="guideMeta" tone="secondary">
            {hint}
          </Text>
        </View>
      ) : null}
    </View>
  );
};

const PlayerTopRow = ({
  audioOpen,
  fullscreen,
  onAudio,
  onFullscreen,
  onLeave,
  onSubtitles,
  pointer,
  subtitlesOn,
  subtitlesOpen,
}: PlayerTopRowProps) => {
  return (
    <View style={{ alignItems: "center", flexDirection: "row", gap: pointer ? 32 : 16 }}>
      {onSubtitles || onAudio ? (
        <View style={{ alignItems: "center", flexDirection: "row", gap: 16 }}>
          {onSubtitles ? (
            <TopKey
              active={subtitlesOn}
              hint="C"
              icon="captions"
              label="Subtitles"
              onPress={onSubtitles}
              pointer={pointer}
              pressed={subtitlesOpen}
              text="Subtitles"
            />
          ) : null}
          {onAudio ? (
            <TopKey
              hint="A"
              icon="audio"
              label="Audio"
              onPress={onAudio}
              pointer={pointer}
              pressed={audioOpen}
              text="Audio"
            />
          ) : null}
        </View>
      ) : null}
      <View style={{ alignItems: "center", flexDirection: "row", gap: 16 }}>
        {onFullscreen && pointer ? (
          <TopKey
            hint="F"
            icon={fullscreen ? "fullscreenExit" : "fullscreen"}
            label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
            onPress={onFullscreen}
            pointer={pointer}
          />
        ) : null}
        <TopKey
          edge={playerChrome.live}
          hint="Esc"
          icon="close"
          label="Leave"
          onPress={onLeave}
          pointer={pointer}
          text="Leave"
        />
      </View>
    </View>
  );
};

export { PlayerTopRow };
