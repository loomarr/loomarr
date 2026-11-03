import { Icon, icons, playerChrome, Text } from "@loomarr/design-system";
import { ScrollView, View } from "react-native";

import { webOnly } from "../glass";
import type { PlayerSheetProps } from "../player-chrome.type";
import { ToolButton } from "../player-transport";

// The one right-hand sheet (§9.1, plane 5): Settings, Info, Channels and the rest all open here, over
// a dimmed picture, full height on the frame's right edge, so the picture stays in view beside it. On
// a portrait phone the same sheet is drawn inline, in the panel's place under the picture.

const PlayerSheet = ({
  channelLabel,
  children,
  density,
  icon,
  inline = false,
  onClose,
  remoteLandsInBody = false,
  title,
}: PlayerSheetProps) => (
  <View
    aria-label={title}
    role="dialog"
    style={
      inline
        ? {
            backgroundColor: playerChrome.sheet,
            borderTopColor: playerChrome.glassRing,
            borderTopWidth: 1,
            flex: 1,
            minHeight: 0,
          }
        : {
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
          }
    }
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
      {/* The title keeps its width; on a narrow sheet the channel gives way, cut short. */}
      <View style={{ flexShrink: 0 }}>
        <Text as="h2" density={density} textRole="label" tone="primary">
          {title}
        </Text>
      </View>
      <View style={{ alignItems: "flex-end", flex: 1, minWidth: 0 }}>
        <Text density={density} numberOfLines={1} textRole="guideMeta" tone="muted">
          {channelLabel}
        </Text>
      </View>
      <ToolButton
        density={density}
        icon="close"
        label={`Close ${title}`}
        onPress={onClose}
        pointer={false}
        preferredFocus={!remoteLandsInBody}
      />
    </View>
    <ScrollView contentContainerStyle={{ padding: 16 }} style={{ flex: 1 }}>
      {children}
    </ScrollView>
  </View>
);

export { PlayerSheet };
