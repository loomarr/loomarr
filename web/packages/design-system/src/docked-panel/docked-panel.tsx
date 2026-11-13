import { View } from "@tamagui/core";
import { Platform, Pressable, Text } from "react-native";

import { Icon } from "../icon";
import { icons } from "../icons";
import { nativeFontFace, playerChrome, semanticTargets, semanticThemes } from "../tokens";
import type { DockedPanelProps } from "./docked-panel.type";

// The native adapter: an absolutely positioned View inside its container (the player's frame),
// which must be a positioned ancestor. It is not a Modal: no scrim, no focus trap, and the picture
// beside it keeps playing. There is no Escape key on a phone or a remote, so the header's close
// button and the screen reader's escape gesture close it; a host that wants the remote's Back to
// close it handles Back itself. It wears the player's chrome in both themes, as the web one does.

const DEFAULT_WIDTH = 372;
const Z_INDEX = 40;
const dark = semanticThemes.dark;
const os = Platform.OS === "android" ? "android" : "ios";

const DockedPanel = ({
  children,
  closeLabel,
  density = "touch",
  edge = "right",
  meta,
  onClose,
  title,
  width = DEFAULT_WIDTH,
}: DockedPanelProps) => {
  const target = density === "tv" ? semanticTargets.tv : semanticTargets.touch;
  return (
    <View
      accessibilityLabel={title}
      backgroundColor={playerChrome.panel}
      borderColor={dark.border.decorative}
      borderLeftWidth={edge === "right" ? 1 : 0}
      borderRightWidth={edge === "left" ? 1 : 0}
      bottom={0}
      left={edge === "left" ? 0 : undefined}
      maxWidth="100%"
      onAccessibilityEscape={onClose}
      position="absolute"
      right={edge === "right" ? 0 : undefined}
      role="dialog"
      top={0}
      width={width}
      zIndex={Z_INDEX}
    >
      <View
        alignItems="center"
        borderBottomColor={dark.surface.elevated}
        borderBottomWidth={1}
        flexDirection="row"
        gap={10}
        paddingBottom={12}
        paddingHorizontal={18}
        paddingTop={16}
      >
        <Text
          accessibilityRole="header"
          style={{
            color: playerChrome.text,
            fontFamily: nativeFontFace("body", os, 600),
            fontSize: 13,
            lineHeight: 20,
          }}
        >
          {title}
        </Text>
        {meta === undefined ? null : (
          <Text
            style={{ color: playerChrome.textMuted, fontFamily: nativeFontFace("data", os), fontSize: 11 }}
          >
            {meta}
          </Text>
        )}
        <Pressable
          accessibilityLabel={closeLabel}
          accessibilityRole="button"
          onPress={onClose}
          style={({ focused }) => ({
            alignItems: "center",
            borderColor: focused ? dark.action.focus : "transparent",
            borderRadius: 4,
            borderWidth: 2,
            height: target,
            justifyContent: "center",
            marginLeft: "auto",
            // The header is 20 high; the target's extra height hangs outside it, so the bar keeps its size.
            marginVertical: -(target - 20) / 2,
            width: target,
          })}
        >
          <Icon color={playerChrome.textMuted} decorative glyph={icons.close} size="compact" />
        </Pressable>
      </View>
      <View flex={1} minHeight={0}>
        {children}
      </View>
    </View>
  );
};

export { DockedPanel };
