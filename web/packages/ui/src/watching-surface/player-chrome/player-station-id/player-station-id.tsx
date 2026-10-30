import { type Density, playerChrome, ThemeScope } from "@loomarr/design-system";
import { View } from "react-native";

import { gradientOr } from "../glass";
import { LowerThirdBand } from "../lower-third-band";
import type { PlayerBandData } from "../player-chrome.type";

// The station-ID card on a portrait phone's picture (§9.1, mock "portrait · station-id"): the band
// alone, fuller, over a scrim at the picture's foot. The picture carries nothing else there; the rest
// of the chrome is in the panel under it.

interface PlayerStationIdProps {
  band: PlayerBandData;
  density: Density;
}

const PlayerStationId = ({ band, density }: PlayerStationIdProps) => (
  <ThemeScope theme="dark">
    <View
      pointerEvents="none"
      style={{
        bottom: 0,
        left: 0,
        paddingBottom: 10,
        paddingHorizontal: 12,
        paddingTop: 32,
        position: "absolute",
        right: 0,
        ...gradientOr(`linear-gradient(0deg, ${playerChrome.scrimFoot}, transparent)`, playerChrome.scrimMid),
      }}
    >
      <LowerThirdBand band={band} density={density} ident />
    </View>
  </ThemeScope>
);

export type { PlayerStationIdProps };
export { PlayerStationId };
