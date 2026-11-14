import { Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Pressable, View, type ViewStyle } from "react-native";

import { nativeFocus, raised, webOnly } from "../flat";

// The casting card (§9.7): while the picture plays on an AirPlay receiver, the phone's picture area
// says where it went and gives the way back. It fills the picture area in the chrome's canvas, which
// covers the black video layer, and is drawn under the keys: the band, the channel keys, Pause and
// More keep working, so the phone is the TV's remote.

/** The foot of a phone on its side, left clear of the band and keys so the card's content reads above them. */
const CASTING_LIFT = 190;

/** The Disconnect button's height. */
const DISCONNECT_HEIGHT = 44;

/** What the card calls the receiver until the player can read its name. */
const FALLBACK_RECEIVER = "your TV";

// Disconnect cannot be a call. iOS has no public API for an app to leave an AirPlay route (switching
// the player's external playback off and on again can resume the cast at once, or bring the picture
// back with the sound still on the TV). Only Apple's route picker can, by the viewer choosing
// "iPhone". So, like the AirPlay key, this button only draws its face; the system picker lies clear
// over it and takes the tap, opening Apple's route list with the phone in it.
const Disconnect = ({ routePicker }: { routePicker: ReactNode }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <View style={{ height: DISCONNECT_HEIGHT }}>
      <Pressable
        accessibilityLabel="Disconnect"
        accessibilityRole="button"
        // The picker is the control a screen reader finds; the face is not a second button.
        aria-hidden
        focusable={false}
        {...focusHandlers}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        style={{
          alignItems: "center",
          borderRadius: 6,
          height: DISCONNECT_HEIGHT,
          justifyContent: "center",
          paddingHorizontal: 16,
          ...raised(),
          ...(hovered ? { backgroundColor: playerChrome.surfaceHover } : null),
          ...webOnly({
            outlineColor: playerChrome.signal,
            outlineOffset: 2,
            outlineStyle: focused ? "solid" : "none",
            outlineWidth: 2,
            transition: "background-color 120ms ease",
          }),
          ...nativeFocus(focused),
        }}
      >
        <Text density="touch" strong textRole="label" tone="primary">
          Disconnect
        </Text>
      </Pressable>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{routePicker}</View>
    </View>
  );
};

interface CastingCardProps {
  /** The receiver's name; "your TV" until a later change reads it from the player. */
  castingTo?: string | undefined;
  /** The system's route picker, laid over the Disconnect button; choosing "iPhone" there brings the picture back. */
  routePicker: ReactNode;
  /** Where the card sits: the caller places it over the picture area. */
  style: ViewStyle;
  /** Padding under the content, to lift it clear of the band when the card fills the screen. */
  lift?: number;
}

const CastingCard = ({ castingTo, lift = 0, routePicker, style }: CastingCardProps) => (
  <View
    // Touches outside the button reach the picture beneath, so a tap there still brings the chrome back.
    pointerEvents="box-none"
    style={{
      alignItems: "center",
      backgroundColor: playerChrome.surface,
      justifyContent: "center",
      paddingBottom: lift,
      position: "absolute",
      ...style,
    }}
    testID="player-casting-card"
  >
    <View style={{ alignItems: "center", gap: 10 }}>
      <Icon color={playerChrome.textSecondary} decorative glyph={icons.airplay} size="touch" />
      <Text density="touch" strong textRole="label" tone="primary">
        {`Playing on ${castingTo ?? FALLBACK_RECEIVER}`}
      </Text>
      <Text density="touch" textRole="cardMeta" tone="secondary">
        Your phone is the remote
      </Text>
      <Disconnect routePicker={routePicker} />
    </View>
  </View>
);

export { CASTING_LIFT, CastingCard };
