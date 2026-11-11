import type { ViewportInsets } from "@loomarr/design-system";
import { useState } from "react";
import { type LayoutChangeEvent, View } from "react-native";

import { ChannelBand } from "../channel-band";
import type { PlayerChromeProps } from "../player-chrome.type";
import { PlayerKeys } from "../player-keys";
import { PlayerLeave, PlayerUtilities } from "../player-utilities";
import { UnmutePill } from "../unmute-pill";

// The upright phone's chrome (§9.7, variant J): nothing covers the picture, which the player centres as
// a strip mid-screen. A toolbar sits under the Dynamic Island, in the black above the picture: Leave
// and the rating on the left, the utilities on the right. The band and keys sit in the black below it,
// grounded above the home indicator. The controls stay up (there is no picture to uncover).

const PortraitChrome = ({
  band,
  formatTime,
  insets,
  keys,
  live,
  onGoLive,
  onLeave,
  unmute,
  utilities,
}: Pick<
  PlayerChromeProps,
  "band" | "formatTime" | "keys" | "live" | "onGoLive" | "onLeave" | "unmute" | "utilities"
> & {
  /** The phone's safe areas. */
  insets: ViewportInsets;
}) => {
  const [frame, setFrame] = useState({ height: 0, width: 0 });
  // The centred 16:9 picture's foot, for the unmute pill just under it.
  const pictureFoot = (frame.height + (frame.width * 9) / 16) / 2;
  return (
    <View
      onLayout={(e: LayoutChangeEvent) => setFrame(e.nativeEvent.layout)}
      pointerEvents="box-none"
      style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0, zIndex: 30 }}
    >
      <View
        pointerEvents="box-none"
        style={{
          alignItems: "center",
          flexDirection: "row",
          justifyContent: "space-between",
          left: 12 + insets.left,
          position: "absolute",
          right: 12 + insets.right,
          top: insets.top + 4,
        }}
        testID="player-toolbar"
      >
        {onLeave ? <PlayerLeave onLeave={onLeave} pointer={false} rating={band.rating} /> : <View />}
        <PlayerUtilities {...utilities} />
      </View>
      {unmute && frame.height > 0 ? (
        <View
          pointerEvents="box-none"
          style={{ alignItems: "center", left: 0, position: "absolute", right: 0, top: pictureFoot + 12 }}
        >
          <UnmutePill onPress={unmute} />
        </View>
      ) : null}
      <View
        pointerEvents="box-none"
        style={{
          bottom: 24 + insets.bottom / 2,
          gap: 16,
          left: 8 + insets.left,
          position: "absolute",
          right: 8 + insets.right,
        }}
      >
        <ChannelBand
          band={band}
          density="touch"
          formatTime={formatTime}
          live={live}
          onGoLive={onGoLive}
          portrait
        />
        <View style={{ alignItems: "center" }}>
          <PlayerKeys {...keys} />
        </View>
      </View>
    </View>
  );
};

export { PortraitChrome };
