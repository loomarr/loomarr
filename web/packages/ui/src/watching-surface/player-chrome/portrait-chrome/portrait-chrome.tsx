import { View } from "react-native";

import { ChannelBand } from "../channel-band";
import type { PlayerChromeProps } from "../player-chrome.type";
import { PlayerKeys } from "../player-keys";
import { PlayerTopRow } from "../player-top-row";
import { UnmutePill } from "../unmute-pill";

// The portrait phone's chrome (§9.7, #2775, the maintainer's pick A): the picture is pinned under the
// top row, and the band and keys sit in the black space below it, so nothing covers the picture. The
// controls stay up (there is no picture to uncover), so there is no auto-hide here.

/** Where the pinned picture starts, in px from the top of the frame: the top row, with room round it. */
const PORTRAIT_PICTURE_TOP = 80;

const PortraitChrome = ({
  band,
  formatTime,
  keys,
  live,
  onGoLive,
  top,
  unmute,
}: Pick<PlayerChromeProps, "band" | "formatTime" | "keys" | "live" | "onGoLive" | "top" | "unmute">) => (
  <View
    pointerEvents="box-none"
    style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0, zIndex: 30 }}
  >
    <View
      pointerEvents="box-none"
      style={{
        alignItems: "center",
        flexDirection: "row",
        height: PORTRAIT_PICTURE_TOP,
        justifyContent: "flex-end",
        paddingHorizontal: 16,
      }}
    >
      <PlayerTopRow {...top} />
    </View>
    {/* The picture's place: the player draws the video here, 16:9 across the frame. */}
    <View pointerEvents="none" style={{ aspectRatio: 16 / 9, width: "100%" }} />
    <View pointerEvents="box-none" style={{ gap: 24, paddingBottom: 24, paddingTop: 16 }}>
      {/* Just under the picture, where it can be reached and covers nothing. */}
      {unmute ? (
        <View style={{ alignItems: "center" }}>
          <UnmutePill onPress={unmute} />
        </View>
      ) : null}
      <View style={{ paddingHorizontal: 8 }}>
        <ChannelBand
          band={band}
          density="touch"
          formatTime={formatTime}
          live={live}
          onGoLive={onGoLive}
          portrait
        />
      </View>
      <View style={{ alignItems: "center" }}>
        <PlayerKeys {...keys} />
      </View>
    </View>
  </View>
);

export { PORTRAIT_PICTURE_TOP, PortraitChrome };
