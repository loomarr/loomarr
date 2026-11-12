import { playerChrome, ThemeScope, useViewportInsets } from "@loomarr/design-system";
import { memo, useDeferredValue, useState } from "react";
import { type LayoutChangeEvent, Platform, View } from "react-native";

import { ChannelBand } from "./channel-band";
import { gradientOr, webOnly } from "./flat";
import { IdentBackdrop } from "./ident-backdrop";
import type { PlayerChromeProps } from "./player-chrome.type";
import { PlayerKeys } from "./player-keys";
import { PlayerReadout } from "./player-readout";
import { PlayerLeave, PlayerUtilities, UTILITY_SIZE } from "./player-utilities";
import { PortraitChrome } from "./portrait-chrome";
import { TuneDip } from "./tune-dip";
import { UnmutePill } from "./unmute-pill";
import { clearOfWatermark, type Frame } from "./watermark-clearance";

// Hidden chrome on the web stays in the tab order and the accessibility tree: focusing a key is how
// a keyboard brings the controls back (the player reveals on focus), so the layer must not be
// aria-hidden over focusable keys (WCAG 4.1.2, axe aria-hidden-focus). Natively, a hidden layer is
// hidden from the reader as before.
const hiddenFromReaders = (hidden: boolean): true | undefined =>
  hidden && Platform.OS !== "web" ? true : undefined;

// How far the readout reaches down from its top, for keeping it clear of a top-right watermark.
const READOUT_REACH = { pointer: 150, touch: 100 } as const;

// PlayerChrome — the chrome every Loomarr player wears (frontend-design §9; the mock of record is
// design/watch-player, variant J). Drawn over the picture, back to front: one bottom scrim; the band
// bottom left; the control cluster bottom right (the keys, with the utilities in a smaller row over
// them); Leave, a back arrow, in the top left corner; then what the keys open. Presentational: the app
// owns the player, the data and the input mapping. It is dark in a light app too: it sits on a
// picture, not on a page.

const ChromeBody = memo(
  ({
    band,
    density,
    formatTime,
    ident = false,
    keys,
    layout = "overlay",
    live,
    menu,
    onGoLive,
    onLeave,
    panel,
    readout,
    schedule,
    sheet,
    unmute,
    utilities,
    visible,
    watermark,
  }: PlayerChromeProps) => {
    const tv = layout === "tv";
    const touch = density === "touch";
    const pointer = keys.pointer;
    // The station-ID card stands in for hidden controls: the band alone, fuller.
    const card = ident && !visible && !sheet;
    const controls = visible;
    const shown = controls || card;
    // A television keeps 5% clear at every edge (overscan): 48 × 27 at 960 × 540. A phone keeps clear
    // of its safe areas: the Dynamic Island, the home indicator and, on its side, both ends (§9.7).
    const insets = useViewportInsets();
    const side = (tv ? 48 : touch ? 24 : 48) + Math.max(insets.left, insets.right);
    const topBase = (tv ? 28 : touch ? 16 : 40) + insets.top;
    const bottomBase = (tv ? 28 : touch ? 16 : 40) + insets.bottom / 2;
    // The channel's watermark is part of the picture, so only the chrome in its corner steps clear of
    // it (§9.6, #3397): top left, Leave and the rating; top right, the readout; bottom left, the band;
    // bottom right, the control cluster. Nothing else moves.
    const [frame, setFrame] = useState<Frame>({ height: 0, width: 0 });
    const [bandHeight, setBandHeight] = useState(0);
    const [clusterHeight, setClusterHeight] = useState(0);
    const leaveAt = clearOfWatermark({
      base: topBase,
      edge: "top",
      extent: pointer ? UTILITY_SIZE.pointer : UTILITY_SIZE.touch,
      frame,
      side: "left",
      watermark,
    });
    const readoutAt = clearOfWatermark({
      base: topBase,
      edge: "top",
      extent: touch ? READOUT_REACH.touch : READOUT_REACH.pointer,
      frame,
      side: "right",
      watermark,
    });
    const liftOf = (extent: number, corner: "left" | "right") =>
      clearOfWatermark({ base: bottomBase, edge: "bottom", extent, frame, side: corner, watermark }) -
      bottomBase;
    const bandLift = liftOf(bandHeight, "left");
    const clusterLift = liftOf(clusterHeight, "right");

    // The open sheet. A phone held upright's rises from the bottom over a picture left undimmed, so a
    // choice shows as it is made; otherwise the right-hand sheet dims the picture.
    if (layout === "portrait")
      return (
        <ThemeScope theme="dark">
          <PortraitChrome
            band={band}
            formatTime={formatTime}
            insets={insets}
            keys={keys}
            live={live}
            onGoLive={onGoLive}
            onLeave={onLeave}
            unmute={unmute}
            utilities={utilities}
          />
          {sheet}
        </ThemeScope>
      );
    const sheetLayer = sheet ? (
      <>
        <View
          pointerEvents="none"
          style={{
            backgroundColor: playerChrome.dim,
            bottom: 0,
            left: 0,
            position: "absolute",
            right: 0,
            top: 0,
            zIndex: 39,
          }}
        />
        {sheet}
      </>
    ) : null;
    return (
      <ThemeScope theme="dark">
        <View
          onLayout={(e: LayoutChangeEvent) => setFrame(e.nativeEvent.layout)}
          pointerEvents="none"
          style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
        />
        <TuneDip tuneKey={band.channelNumber} />
        {card && band.backdrop && frame.width > 0 ? (
          <IdentBackdrop art={band.backdrop} frame={frame} />
        ) : null}
        {/* The scrim passes touches through to the picture but not its keys: native's "none" would
            silence every key under it whatever they say, where the web lets a child take them back. */}
        <View
          aria-hidden={hiddenFromReaders(!shown)}
          pointerEvents="box-none"
          style={{
            bottom: 0,
            left: 0,
            opacity: shown ? 1 : 0,
            paddingTop: touch ? 40 : 96,
            position: "absolute",
            right: 0,
            zIndex: 30,
            ...gradientOr(
              `linear-gradient(0deg, ${playerChrome.scrimFoot}, ${playerChrome.scrimMid} 55%, transparent)`,
              playerChrome.scrimMid,
            ),
            ...webOnly({ transition: "opacity 120ms ease" }),
          }}
        >
          <View
            pointerEvents={shown ? "box-none" : "none"}
            style={{
              alignItems: "flex-end",
              flexDirection: "row",
              gap: tv ? 28 : touch ? 24 : 48,
              paddingBottom: bottomBase,
              paddingHorizontal: side,
            }}
          >
            <View
              onLayout={(e: LayoutChangeEvent) => setBandHeight(e.nativeEvent.layout.height)}
              style={{ flex: 1, marginBottom: bandLift, minWidth: 0 }}
            >
              <ChannelBand
                band={band}
                density={density}
                formatTime={formatTime}
                ident={card}
                live={live}
                onGoLive={onGoLive}
                schedule={schedule}
                tv={tv}
              />
            </View>
            {card ? null : (
              // The control cluster: everything you can press, in one place. The utilities are a smaller
              // row over the main keys, sharing their right edge (§9.3). A television has none: its
              // Subtitles and Audio are main keys, and the remote has the rest. What a key opens is a
              // menu over them all (§9.5), so the rest of the chrome stays where it is.
              <View
                aria-hidden={hiddenFromReaders(!controls)}
                onLayout={(e: LayoutChangeEvent) => setClusterHeight(e.nativeEvent.layout.height)}
                pointerEvents={controls ? "box-none" : "none"}
                style={{
                  alignItems: "flex-end",
                  flexShrink: 0,
                  gap: pointer ? 12 : 8,
                  marginBottom: clusterLift,
                }}
                testID="player-cluster"
              >
                {menu}
                {tv ? null : <PlayerUtilities {...utilities} />}
                <PlayerKeys {...keys} {...(tv ? { audio: utilities } : {})} />
              </View>
            )}
          </View>
        </View>
        {onLeave && !tv ? (
          // Leave: the top left corner, where people reach to leave (§9.3). The rating sits beside it.
          // A television has none: the remote's Back leaves.
          <View
            aria-hidden={hiddenFromReaders(!controls)}
            pointerEvents={controls ? "box-none" : "none"}
            style={{
              left: side,
              opacity: controls ? 1 : 0,
              position: "absolute",
              top: leaveAt,
              zIndex: 40,
              ...webOnly({ transition: "opacity 120ms ease" }),
            }}
          >
            <PlayerLeave onLeave={onLeave} pointer={pointer} rating={band.rating} />
          </View>
        ) : null}
        {unmute ? (
          // The muted first play: top centre, clear of both corners.
          <View
            pointerEvents="box-none"
            style={{
              alignItems: "center",
              left: 0,
              position: "absolute",
              right: 0,
              top: topBase,
              zIndex: 40,
            }}
          >
            <UnmutePill onPress={unmute} />
          </View>
        ) : null}
        {readout ? <PlayerReadout density={density} top={readoutAt} {...readout} /> : null}
        {panel}
        {sheetLayer}
      </ThemeScope>
    );
  },
);

// The chrome follows a deferred copy of its props. A channel press changes nearly all of them while
// the chrome is on its way out, and rendering this tree then would hold up the tuning overlay's paint.
const PlayerChrome = (props: PlayerChromeProps) => <ChromeBody {...useDeferredValue(props)} />;

export { PlayerChrome };
