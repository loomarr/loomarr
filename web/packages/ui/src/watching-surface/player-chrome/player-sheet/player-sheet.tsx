import { Icon, icons, playerChrome, Text } from "@loomarr/design-system";
import { ScrollView, View } from "react-native";

import { webOnly } from "../glass";
import type { PlayerSheetProps } from "../player-chrome.type";
import { ToolButton } from "../player-transport";

// The one right-hand sheet (§9.1, plane 5): Settings, Info, Channels and the rest all open here, over
// a dimmed picture, full height on the frame's right edge, so the picture stays in view beside it.

const PlayerSheet = ({ channelLabel, children, density, icon, onClose, title }: PlayerSheetProps) => (
  <View
    aria-label={title}
    role="dialog"
    style={{
      backgroundColor: playerChrome.sheet,
      borderLeftColor: playerChrome.glassRing,
      borderLeftWidth: 1,
      bottom: 0,
      maxWidth: "100%",
      position: "absolute",
      right: 0,
      top: 0,
      width: density === "touch" ? 384 : 416,
      zIndex: 40,
      ...webOnly({
        backdropFilter: `blur(${playerChrome.glassBlur}px)`,
        boxShadow: playerChrome.shadowCard,
      }),
    }}
  >
    <View
      style={{
        alignItems: "center",
        borderBottomColor: playerChrome.glassRing,
        borderBottomWidth: 1,
        flexDirection: "row",
        gap: 10,
        paddingLeft: 16,
        paddingRight: 8,
        paddingVertical: 6,
      }}
    >
      <Icon color={playerChrome.signal} decorative glyph={icons[icon]} size="compact" />
      <View style={{ flex: 1 }}>
        <Text as="h2" density={density} textRole="label" tone="primary">
          {title}
        </Text>
      </View>
      <Text density={density} numberOfLines={1} textRole="guideMeta" tone="muted">
        {channelLabel}
      </Text>
      <ToolButton density={density} icon="close" label={`Close ${title}`} onPress={onClose} pointer={false} />
    </View>
    <ScrollView contentContainerStyle={{ padding: 16 }} style={{ flex: 1 }}>
      {children}
    </ScrollView>
  </View>
);

export { PlayerSheet };
