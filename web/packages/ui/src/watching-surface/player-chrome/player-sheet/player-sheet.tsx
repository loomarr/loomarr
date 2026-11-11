import { Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useCallback, useState } from "react";
import { Pressable, ScrollView, View } from "react-native";

import { BarsEdge, ChannelBlock } from "../channel-band";
import { nativeFocus, webOnly } from "../flat";
import type { PlayerSheetProps } from "../player-chrome.type";

// The one right-hand sheet (§9.5, layer 4): Subtitles, Audio, Picture, More, Info and Channels all
// open here, over a dimmed picture, full height on the frame's right edge. It wears the band's
// identity: the seven-bar edge, a compact channel block and the channel's name in the header.

const CloseKey = ({ title, onPress }: { title: string; onPress: () => void }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  // Focus moves into the sheet when it opens, so Esc and Tab act on it, not on a key that just left.
  // A stable callback ref runs once, on mount. (Native has no focus to move; the web's host element does.)
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
        backgroundColor: hovered ? playerChrome.surfaceHover : playerChrome.surfaceRaised,
        borderColor: playerChrome.hairline,
        borderRadius: 6,
        borderWidth: 1,
        height: 48,
        justifyContent: "center",
        width: 48,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
        ...nativeFocus(focused),
      }}
    >
      <Icon color={playerChrome.text} decorative glyph={icons.close} size="control" />
    </Pressable>
  );
};

const PlayerSheet = ({ channel, children, density, onClose, title }: PlayerSheetProps) => (
  <View
    aria-label={title}
    role="dialog"
    style={{
      backgroundColor: playerChrome.surface,
      borderLeftColor: playerChrome.hairline,
      borderLeftWidth: 1,
      bottom: 0,
      maxWidth: "100%",
      position: "absolute",
      right: 0,
      top: 0,
      width: density === "touch" ? 384 : 416,
      zIndex: 40,
    }}
  >
    <BarsEdge height={density === "touch" ? 8 : 10} />
    <View
      style={{
        alignItems: "stretch",
        borderBottomColor: playerChrome.hairline,
        borderBottomWidth: 1,
        flexDirection: "row",
        gap: 16,
        paddingRight: 16,
      }}
    >
      <ChannelBlock density="touch" number={channel.number} width={72} />
      <View style={{ flex: 1, gap: 4, justifyContent: "center", minWidth: 0, paddingVertical: 16 }}>
        <Text density={density} numberOfLines={1} textRole="label" tone="secondary">
          {channel.name}
        </Text>
        <Text as="h2" density={density} numberOfLines={1} strong textRole="title" tone="primary">
          {title}
        </Text>
      </View>
      <View style={{ justifyContent: "center" }}>
        <CloseKey onPress={onClose} title={title} />
      </View>
    </View>
    <ScrollView
      contentContainerStyle={{ paddingBottom: 32, paddingHorizontal: 24, paddingTop: 16 }}
      style={{ flex: 1 }}
    >
      {children}
    </ScrollView>
  </View>
);

export { PlayerSheet };
