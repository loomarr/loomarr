import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { panelGlass, webOnly } from "../glass";
import type { PlayerBandData } from "../player-chrome.type";

// The lower third (§9.1, plane 2): identity. An amber channel block — CH over the number — beside a
// glass panel with `CH n · name`, the programme line, a thin progress line and the time. While a
// preview card is open the band steps back a plane, dimmer and softened, so the card is in front.

interface LowerThirdBandProps {
  band: PlayerBandData;
  density: Density;
  /** A preview card is open in front of it. */
  steppedBack?: boolean;
}

const LowerThirdBand = ({ band, density, steppedBack = false }: LowerThirdBandProps) => {
  const compact = density === "touch";
  return (
    <View
      style={{
        alignItems: "stretch",
        flexDirection: "row",
        maxWidth: 960,
        opacity: steppedBack ? 0.4 : 1,
        ...webOnly({
          filter: steppedBack ? "blur(1px)" : "none",
          transition: "opacity 220ms ease, filter 220ms ease",
        }),
      }}
    >
      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        style={{
          alignItems: "center",
          backgroundColor: playerChrome.signal,
          borderBottomLeftRadius: 8,
          borderTopLeftRadius: 8,
          justifyContent: "center",
          paddingVertical: 6,
          width: compact ? 48 : 64,
          ...webOnly({
            backgroundImage: `linear-gradient(180deg, ${playerChrome.signal}, ${playerChrome.signalDeep})`,
            boxShadow: "inset 0 1px 0 rgba(255, 255, 255, 0.45), inset 0 -2px 0 rgba(0, 0, 0, 0.15)",
          }),
        }}
      >
        <Text density={density} shout textRole="guideIdent" tone="inverse" tracking={0.2}>
          CH
        </Text>
        <Text density={density} textRole={compact ? "guideNumber" : "channelNumber"} tone="inverse">
          {band.channelNumber}
        </Text>
      </View>
      <View
        style={{
          borderBottomRightRadius: 8,
          borderTopRightRadius: 8,
          flex: 1,
          gap: 2,
          minWidth: 0,
          paddingHorizontal: 16,
          paddingVertical: 10,
          ...panelGlass(),
        }}
      >
        <Text density={density} numberOfLines={1} shout textRole="guideMeta" tone="secondary" tracking={0.14}>
          {`CH ${band.channelNumber} · ${band.channelName}`}
        </Text>
        {band.programmeLine ? (
          <Text
            density={density}
            numberOfLines={1}
            strong
            textRole={compact ? "cardTitle" : "headline"}
            tone="primary"
          >
            {band.programmeLine}
          </Text>
        ) : null}
        {band.progress !== undefined || band.timeLabel ? (
          <View style={{ alignItems: "center", flexDirection: "row", gap: 12, marginTop: 6 }}>
            <View
              style={{
                backgroundColor: playerChrome.track,
                borderRadius: 2,
                flex: 1,
                height: 4,
                overflow: "hidden",
              }}
            >
              <View
                style={{
                  backgroundColor: band.inBreak ? playerChrome.breakAccent : playerChrome.signal,
                  height: "100%",
                  width: `${Math.round((band.progress ?? 0) * 100)}%`,
                }}
              />
            </View>
            {band.timeLabel ? (
              <Text density={density} textRole="cardTime" tone="secondary">
                {band.timeLabel}
              </Text>
            ) : null}
          </View>
        ) : null}
      </View>
    </View>
  );
};

export type { LowerThirdBandProps };
export { LowerThirdBand };
