import { playerChrome, ThemeScope } from "@loomarr/design-system";
import { memo, useDeferredValue } from "react";
import { View } from "react-native";

import { ChannelBand } from "./channel-band";
import { gradientOr, webOnly } from "./flat";
import type { PlayerChromeProps } from "./player-chrome.type";
import { PlayerKeys } from "./player-keys";
import { PlayerTopRow } from "./player-top-row";

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
    live,
    onGoLive,
    sheet,
    top,
    visible,
  }: PlayerChromeProps) => {
    const touch = density === "touch";
    // The station-ID card stands in for hidden controls: the band alone, fuller.
    const card = ident && !visible && !sheet;
    const controls = visible;
    const shown = controls || card;
    const edge = touch ? 16 : 48;
    return (
      <ThemeScope theme="dark">
        <View
          aria-hidden={!shown || undefined}
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
            pointerEvents={shown ? "box-none" : "none"}
            style={{
              alignItems: "flex-end",
              flexDirection: "row",
              gap: touch ? 32 : 64,
              paddingBottom: touch ? 16 : 48,
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
          aria-hidden={!controls || undefined}
          pointerEvents={controls ? "box-none" : "none"}
          style={{
            opacity: controls ? 1 : 0,
            position: "absolute",
            right: touch ? 16 : 48,
            top: touch ? 16 : 40,
            zIndex: 40,
            ...webOnly({ transition: "opacity 120ms ease" }),
          }}
        >
          <PlayerTopRow {...top} />
        </View>
        {sheet ? (
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
        ) : null}
      </ThemeScope>
    );
  },
);

// The chrome follows a deferred copy of its props. A channel press changes nearly all of them while
// the chrome is on its way out, and rendering this tree then would hold up the tuning overlay's paint.
const PlayerChrome = (props: PlayerChromeProps) => <ChromeBody {...useDeferredValue(props)} />;

export { PlayerChrome };
