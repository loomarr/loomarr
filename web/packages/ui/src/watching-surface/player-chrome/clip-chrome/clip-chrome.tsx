import { playerChrome, Text, ThemeScope } from "@loomarr/design-system";
import { View } from "react-native";

import { gradientOr, webOnly } from "../glass";
import type { ClipChromeProps } from "../player-chrome.type";
import { ClipTransport } from "../player-transport";

// A clip's chrome over its picture: `leading` and the title on a top scrim, the transport on a
// bottom one, both fading together with the player's auto-hide. The live chrome's scrim and
// pieces at a clip player's smaller size, in the fixed dark theme because it sits on a picture.
// An untitled clip with nothing leading draws no top scrim at all, rather than an empty band.
const ClipChrome = ({ leading, shown, title, ...transport }: ClipChromeProps) => (
  <ThemeScope theme="dark">
    {title || leading ? (
      <View
        aria-hidden={!shown || undefined}
        pointerEvents={shown ? "box-none" : "none"}
        style={{
          alignItems: "center",
          flexDirection: "row",
          gap: 10,
          left: 0,
          opacity: shown ? 1 : 0,
          padding: 12,
          position: "absolute",
          right: 0,
          top: 0,
          ...gradientOr(
            `linear-gradient(180deg, ${playerChrome.scrimMid}, transparent)`,
            playerChrome.scrimMid,
          ),
          ...webOnly({ transition: "opacity 200ms ease" }),
        }}
        testID="clip-chrome-top"
      >
        <View style={{ flexShrink: 0 }}>{leading}</View>
        {title ? (
          <Text numberOfLines={1} strong style={{ flexShrink: 1, marginLeft: "auto" }} textRole="body">
            {title}
          </Text>
        ) : null}
      </View>
    ) : null}
    <View
      aria-hidden={!shown || undefined}
      pointerEvents={shown ? "box-none" : "none"}
      style={{
        bottom: 0,
        left: 0,
        opacity: shown ? 1 : 0,
        paddingBottom: 12,
        paddingHorizontal: 16,
        paddingTop: 32,
        position: "absolute",
        right: 0,
        ...gradientOr(
          `linear-gradient(0deg, ${playerChrome.scrimFoot}, ${playerChrome.scrimMid} 55%, transparent)`,
          playerChrome.scrimMid,
        ),
        ...webOnly({ transition: "opacity 200ms ease" }),
      }}
    >
      <ClipTransport {...transport} />
    </View>
  </ThemeScope>
);

export { ClipChrome };
