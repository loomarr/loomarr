import {
  Icon,
  icons,
  NonModalDialog,
  playerChrome,
  Text,
  useFocusLook,
  useViewportInsets,
} from "@loomarr/design-system";
import { type ReactNode, useCallback, useState } from "react";
import { Pressable, ScrollView, View } from "react-native";

import { nativeFocus, webOnly } from "../flat";
import { UTILITY_SIZE } from "../player-utilities";

// The bottom sheet (§9.5, variant J): what a key opens on a phone held upright. It rises from the
// bottom as tall as what it holds, so the picture above stays in view (subtitles change as you choose
// them), and never reaches over the toolbar at the top of the screen.

interface PlayerBottomSheetProps {
  children: ReactNode;
  onClose: () => void;
  title: string;
}

// The toolbar sits 4 below the top safe area and is a touch key tall; the sheet stops 12 short of it.
const TOOLBAR_CLEARANCE = 4 + UTILITY_SIZE.touch + 12;

const CloseKey = ({ onPress, title }: { onPress: () => void; title: string }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  // Focus moves into the sheet when it opens, so Esc and Tab act on it, not on a key that just left.
  const focusOnMount = useCallback((el: unknown) => {
    (el as { focus?: () => void } | null)?.focus?.();
  }, []);
  return (
    <Pressable
      ref={focusOnMount}
      accessibilityLabel={`Close ${title}`}
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: hovered ? playerChrome.surfaceHover : "transparent",
        borderRadius: 10,
        height: 44,
        justifyContent: "center",
        width: 44,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineOffset: -2,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
        ...nativeFocus(focused),
      }}
    >
      <Icon color={playerChrome.textSecondary} decorative glyph={icons.close} size="default" />
    </Pressable>
  );
};

const PlayerBottomSheet = ({ children, onClose, title }: PlayerBottomSheetProps) => {
  const insets = useViewportInsets();
  return (
    <View
      pointerEvents="box-none"
      style={{
        bottom: 0,
        justifyContent: "flex-end",
        left: 0,
        position: "absolute",
        right: 0,
        top: insets.top + TOOLBAR_CLEARANCE,
        zIndex: 40,
      }}
    >
      <NonModalDialog
        label={title}
        style={{
          backgroundColor: playerChrome.band,
          borderTopColor: playerChrome.hairline,
          borderTopLeftRadius: 16,
          borderTopRightRadius: 16,
          borderTopWidth: 1,
          flexShrink: 1,
          paddingBottom: insets.bottom,
        }}
      >
        <View
          aria-hidden
          style={{
            alignSelf: "center",
            backgroundColor: playerChrome.track,
            borderRadius: 2,
            height: 4,
            marginTop: 6,
            width: 36,
          }}
        />
        <View style={{ alignItems: "center", flexDirection: "row", paddingLeft: 20, paddingRight: 8 }}>
          <Text as="h2" density="touch" strong style={{ flex: 1 }} textRole="label" tone="primary">
            {title}
          </Text>
          <CloseKey onPress={onClose} title={title} />
        </View>
        <ScrollView
          contentContainerStyle={{ paddingBottom: 12, paddingHorizontal: 8 }}
          style={{ flexShrink: 1 }}
        >
          {children}
        </ScrollView>
      </NonModalDialog>
    </View>
  );
};

export type { PlayerBottomSheetProps };
export { PlayerBottomSheet };
