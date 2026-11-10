import { playerChrome, ThemeScope } from "@loomarr/design-system";
import { memo, useDeferredValue, useState } from "react";
import { type LayoutChangeEvent, Platform, View } from "react-native";

import { ChannelBand } from "./channel-band";
import { gradientOr, webOnly } from "./flat";
import { IdentBackdrop } from "./ident-backdrop";
import type { PlayerChromeProps } from "./player-chrome.type";
import { PlayerKeys } from "./player-keys";
import { PlayerReadout } from "./player-readout";
import { PlayerTopRow } from "./player-top-row";
import { PortraitChrome } from "./portrait-chrome";
import { TuneDip } from "./tune-dip";
import { UNMUTE_PILL_HEIGHT, UnmutePill } from "./unmute-pill";
import { clearOfWatermark, type Frame } from "./watermark-clearance";

// The height of a top-row key.
const KEY = 48;

// Hidden chrome on the web stays in the tab order and the accessibility tree: focusing a key is how
// a keyboard brings the controls back (the player reveals on focus), so the layer must not be
// aria-hidden over focusable keys (WCAG 4.1.2, axe aria-hidden-focus). Natively, a hidden layer is
// hidden from the reader as before.
const hiddenFromReaders = (hidden: boolean): true | undefined =>
  hidden && Platform.OS !== "web" ? true : undefined;

// PlayerChrome — the chrome every Loomarr player wears (frontend-design §9; the mock of record is
// design/watch-player, variant H). Drawn over the picture, back to front: one bottom scrim, the band
// and the keys beside it, the top row at the upper right; then the transient layer, one right-hand
// sheet over a dimmed picture. Presentational: the app owns the player, the data and the input
// mapping. It is dark in a light app too — it sits on a picture, not on a page.

const ChromeBody = memo(
  ({
    band,
    density,
    formatTime,
    ident = false,
    keys,
    layout = "overlay",
    live,
    onGoLive,
    readout,
    panel,
    sheet,
    top,
    unmute,
    visible,
    watermark,
  }: PlayerChromeProps) => {
    const touch = density === "touch";
    // The station-ID card stands in for hidden controls: the band alone, fuller.
    const card = ident && !visible && !sheet;
    const controls = visible;
    const shown = controls || card;
    const edge = touch ? 16 : 48;
    // The channel's watermark is part of the picture, so the chrome steps around it (§9.6): the top
    // row and the readout (upper right) start below a top-right bug, the unmute pill (upper left)
    // below a top-left one, and the band and keys, which span the frame, lift clear of either bottom
    // bug. Only the chrome on the bug's corner moves (#3397); the two top corners may then sit at
    // different heights, as the mock draws them.
    const [frame, setFrame] = useState<Frame>({ height: 0, width: 0 });
    const [rowHeight, setRowHeight] = useState(0);
    const topBase = touch ? 16 : 40;
    const topRowAt = clearOfWatermark({
      base: topBase,
      edge: "top",
      extent: KEY,
      frame,
      side: "right",
      watermark,
    });
    const pillAt = clearOfWatermark({
      base: topBase,
      edge: "top",
      extent: UNMUTE_PILL_HEIGHT,
      frame,
      side: "left",
      watermark,
    });
    const bottomBase = touch ? 16 : 48;
    const bottomAt = clearOfWatermark({
      base: bottomBase,
      edge: "bottom",
      extent: rowHeight,
      frame,
      side: "both",
      watermark,
    });
    // The open sheet, over a dimmed picture, above either layout.
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
    if (layout === "portrait")
      return (
        <ThemeScope theme="dark">
          <PortraitChrome
            band={band}
            formatTime={formatTime}
            keys={keys}
            live={live}
            onGoLive={onGoLive}
            top={top}
            unmute={unmute}
          />
          {sheetLayer}
        </ThemeScope>
      );
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
        <View
          aria-hidden={hiddenFromReaders(!shown)}
          pointerEvents="none"
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
            onLayout={(e: LayoutChangeEvent) => setRowHeight(e.nativeEvent.layout.height - bottomAt)}
            pointerEvents={shown ? "box-none" : "none"}
            style={{
              alignItems: "flex-end",
              flexDirection: "row",
              gap: touch ? 32 : 64,
              paddingBottom: bottomAt,
              paddingHorizontal: edge,
            }}
          >
            <View style={{ flex: 1, minWidth: 0 }}>
              <ChannelBand
                band={band}
                density={density}
                formatTime={formatTime}
                ident={card}
                live={live}
                onGoLive={onGoLive}
              />
            </View>
            {card ? null : <PlayerKeys {...keys} />}
          </View>
        </View>
        <View
          aria-hidden={hiddenFromReaders(!controls)}
          pointerEvents={controls ? "box-none" : "none"}
          style={{
            opacity: controls ? 1 : 0,
            position: "absolute",
            right: touch ? 16 : 48,
            top: topRowAt,
            zIndex: 40,
            ...webOnly({ transition: "opacity 120ms ease" }),
          }}
        >
          <PlayerTopRow {...top} />
        </View>
        {unmute ? (
          <View style={{ left: edge, position: "absolute", top: pillAt, zIndex: 40 }}>
            <UnmutePill onPress={unmute} />
          </View>
        ) : null}
        {readout ? (
          <PlayerReadout
            density={density}
            digits={readout.digits}
            missed={readout.missed}
            // Under the top row while the controls are up; in its place while they are hidden.
            top={controls ? topRowAt + KEY + (touch ? 16 : 24) : topRowAt}
          />
        ) : null}
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
