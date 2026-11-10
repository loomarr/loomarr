import { Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable } from "react-native";

import { raised, webOnly } from "../flat";

// The first play in a session starts muted (§9.3), so a channel never blasts at someone who just
// opened it. This pill says so and takes one tap to undo it. It is the player's only chrome that
// stays up while the controls are hidden, and it goes when the sound is on.

/** The pill's height: the chrome uses it to keep the pill clear of a top-left watermark. */
const UNMUTE_PILL_HEIGHT = 40;

const UnmutePill = ({ onPress }: { onPress: () => void }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel="Tap to unmute"
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        borderRadius: 6,
        flexDirection: "row",
        gap: 8,
        height: UNMUTE_PILL_HEIGHT,
        paddingHorizontal: 12,
        ...raised(),
        ...(hovered ? { backgroundColor: playerChrome.surfaceHover } : null),
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineOffset: 2,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
      }}
    >
      <Icon color={playerChrome.text} decorative glyph={icons.volumeMuted} size="compact" />
      <Text density="touch" strong textRole="label" tone="primary">
        Tap to unmute
      </Text>
    </Pressable>
  );
};

export { UNMUTE_PILL_HEIGHT, UnmutePill };
