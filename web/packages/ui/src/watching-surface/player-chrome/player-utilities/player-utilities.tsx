import { Icon, type IconName, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

import { nativeFocus, raised, webOnly } from "../flat";
import type { PlayerUtilitiesProps } from "../player-chrome.type";

// The utilities (§9.3, variant J): a smaller row of icon keys directly over the main keys, sharing
// their right edge, so they are grounded in the control cluster rather than floating under the
// watermark. Subtitles and Audio, then Fullscreen, or AirPlay and picture in picture on an iPhone.
// Leave is the same key, a back arrow, on its own in the top left corner, with the rating beside it.

/** A utility key's side: smaller than a main key (§9.3). */
const UTILITY_SIZE = { pointer: 40, touch: 44 } as const;

interface UtilityKeyProps {
  /** Subtitles on: a small white dot, never amber. */
  active?: boolean | undefined;
  /** The key that does this on a keyboard, named in the tooltip. */
  hint?: string;
  icon: IconName;
  label: string;
  onPress: () => void;
  pointer: boolean;
  /** What it opens is open: a tone lighter, and it says it is expanded. */
  pressed?: boolean | undefined;
  /** The tooltip opens downward (Leave, at the top of the screen); otherwise upward. */
  tipBelow?: boolean;
}

const UtilityKey = ({
  active,
  hint,
  icon,
  label,
  onPress,
  pointer,
  pressed = false,
  tipBelow = false,
}: UtilityKeyProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  const size = pointer ? UTILITY_SIZE.pointer : UTILITY_SIZE.touch;
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
          borderRadius: 10,
          height: size,
          justifyContent: "center",
          width: size,
          ...raised(),
          ...(hovered || down ? { backgroundColor: playerChrome.surfaceHover } : null),
          ...(pressed
            ? { backgroundColor: playerChrome.surfaceHover, borderColor: playerChrome.textSecondary }
            : null),
          ...webOnly({
            outlineColor: playerChrome.signal,
            outlineOffset: 2,
            outlineStyle: focused ? "solid" : "none",
            outlineWidth: 2,
            transition: "background-color 120ms ease, border-color 120ms ease",
          }),
          ...nativeFocus(focused),
        })}
      >
        <Icon
          color={playerChrome.text}
          decorative
          glyph={icons[icon]}
          size={pointer ? "default" : "navigation"}
        />
        {active ? (
          <View
            style={{
              backgroundColor: playerChrome.text,
              borderRadius: 3,
              height: 6,
              position: "absolute",
              right: 5,
              top: 5,
              width: 6,
            }}
          />
        ) : null}
      </Pressable>
      {pointer && (hovered || focused) ? (
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
            ...(tipBelow ? { left: 0, top: size + 8 } : { bottom: size + 8, right: 0 }),
            ...webOnly({ whiteSpace: "nowrap" }),
          }}
        >
          <Text density="pointer" numberOfLines={1} textRole="label" tone="primary">
            {label}
          </Text>
          {hint ? (
            <Text density="pointer" textRole="mono" tone="secondary">
              {hint}
            </Text>
          ) : null}
        </View>
      ) : null}
    </View>
  );
};

const PlayerUtilities = ({
  audioOpen,
  fullscreen,
  onAirPlay,
  onAudio,
  onFullscreen,
  onPictureInPicture,
  onSubtitles,
  pointer,
  subtitlesOn,
  subtitlesOpen,
}: PlayerUtilitiesProps) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 6 }}>
    {onSubtitles ? (
      <UtilityKey
        active={subtitlesOn}
        hint="C"
        icon="captions"
        label="Subtitles"
        onPress={onSubtitles}
        pointer={pointer}
        pressed={subtitlesOpen}
      />
    ) : null}
    {onAudio ? (
      <UtilityKey
        hint="A"
        icon="audio"
        label="Audio"
        onPress={onAudio}
        pointer={pointer}
        pressed={audioOpen}
      />
    ) : null}
    {onAirPlay ? <UtilityKey icon="airplay" label="AirPlay" onPress={onAirPlay} pointer={pointer} /> : null}
    {onPictureInPicture ? (
      <UtilityKey
        icon="pictureInPicture"
        label="Picture in picture"
        onPress={onPictureInPicture}
        pointer={pointer}
      />
    ) : null}
    {onFullscreen ? (
      <UtilityKey
        hint="F"
        icon={fullscreen ? "fullscreenExit" : "fullscreen"}
        label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
        onPress={onFullscreen}
        pointer={pointer}
      />
    ) : null}
  </View>
);

/**
 * Leave (§9.3): a back arrow in the top left corner, where people reach to leave a player, with the
 * programme's rating beside it. A television has no such key; Back leaves.
 */
const PlayerLeave = ({
  onLeave,
  pointer,
  rating,
}: {
  onLeave: () => void;
  pointer: boolean;
  rating?: string | undefined;
}) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 12 }}>
    <UtilityKey hint="Esc" icon="goBack" label="Leave" onPress={onLeave} pointer={pointer} tipBelow />
    {rating ? (
      <View
        style={{
          backgroundColor: playerChrome.scrimMid,
          borderColor: "rgba(255, 255, 255, 0.72)",
          borderRadius: 4,
          borderWidth: 1,
          paddingHorizontal: pointer ? 12 : 8,
          paddingVertical: pointer ? 6 : 4,
        }}
        testID="player-rating"
      >
        <Text density={pointer ? "pointer" : "touch"} strong textRole="mono" tone="primary">
          {rating}
        </Text>
      </View>
    ) : null}
  </View>
);

export { PlayerLeave, PlayerUtilities, UTILITY_SIZE };
