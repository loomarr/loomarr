import { type Density, Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { Pressable, View, type ViewStyle } from "react-native";

import { nativeFocus, webOnly } from "../flat";

// The cluster menu (§9.5, variant J): on desktop and television, what a key opens is a menu in the
// control cluster, right over the keys, 280px wide. One place for everything transient, next to the
// key that opened it, and the picture stays whole. A television moves the amber ring through its
// rows; Back closes it.

interface PlayerMenuProps {
  children: ReactNode;
  density: Density;
  /** The menu's name: its heading, and what it is called to a reader. */
  title: string;
  /** More names no heading: its rows say what they are. */
  heading?: boolean;
  /** A second page (More's Picture): the heading is the way back to the first. */
  onBack?: () => void;
}

const MENU_WIDTH = { pointer: 280, touch: 320, tv: 360 } as const;

/**
 * One row of what the keys open, in a menu or a sheet alike (variant J): 44 high on a pointer, the
 * density's target elsewhere, no fill until hovered, and the amber ring drawn inside the row so a
 * menu's edge never clips it.
 */
const playerRow = (density: Density, look: { focused: boolean; hovered: boolean }): ViewStyle => ({
  alignItems: "center",
  backgroundColor: look.hovered ? playerChrome.surfaceHover : "transparent",
  borderRadius: 8,
  flexDirection: "row",
  gap: 12,
  minHeight: density === "pointer" ? 44 : density === "tv" ? 64 : 48,
  paddingHorizontal: density === "pointer" ? 10 : 12,
  paddingVertical: 6,
  ...webOnly({
    outlineColor: playerChrome.signal,
    outlineOffset: -2,
    outlineStyle: look.focused ? "solid" : "none",
    outlineWidth: 2,
    transition: "background-color 120ms ease",
  }),
  ...nativeFocus(look.focused),
});

/** A row's words: the label voice on a pointer, body elsewhere. */
const rowRole = (density: Density) => (density === "pointer" ? "label" : "body");

const BackHeading = ({ density, onBack, title }: { density: Density; onBack: () => void; title: string }) => {
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={`Back from ${title}`}
      accessibilityRole="button"
      {...focusHandlers}
      onPress={onBack}
      style={{ ...playerRow(density, { focused, hovered: false }), gap: 4, minHeight: 32 }}
    >
      <Icon color={playerChrome.textSecondary} decorative glyph={icons.back} size="compact" />
      <Text density={density} strong textRole="cardMeta" tone="secondary">
        {title}
      </Text>
    </Pressable>
  );
};

const PlayerMenu = ({ children, density, heading = true, onBack, title }: PlayerMenuProps) => (
  // A non-modal dialog, not an ARIA menu: its rows are the radios, switch and buttons they are,
  // reached by Tab and the remote's arrows, with none of a menu's own arrow-key contract. As a dialog
  // it is a layer like the sheets: while it is open the arrows are not the player's to change channel.
  <View
    aria-label={title}
    role="dialog"
    style={{
      backgroundColor: playerChrome.band,
      borderColor: playerChrome.hairline,
      borderRadius: 12,
      borderWidth: 1,
      gap: 2,
      maxWidth: "100%",
      padding: 6,
      width: MENU_WIDTH[density],
    }}
    testID="player-menu"
  >
    {onBack ? (
      <BackHeading density={density} onBack={onBack} title={title} />
    ) : heading ? (
      <View style={{ height: 32, justifyContent: "center", paddingHorizontal: 10 }}>
        <Text density={density} strong textRole="cardMeta" tone="secondary">
          {title}
        </Text>
      </View>
    ) : null}
    {children}
  </View>
);

export type { PlayerMenuProps };
export { PlayerMenu, playerRow, rowRole };
