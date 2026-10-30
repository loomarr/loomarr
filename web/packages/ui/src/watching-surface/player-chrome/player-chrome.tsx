import { playerChrome, ThemeScope } from "@loomarr/design-system";
import { useState } from "react";
import { View } from "react-native";

import { gradientOr, webOnly } from "./glass";
import { LowerThirdBand } from "./lower-third-band";
import type { PlayerChromeProps } from "./player-chrome.type";
import { PlayerTransport } from "./player-transport";
import { PreviewPlaybar } from "./preview-playbar";

// PlayerChrome — the chrome every Loomarr player wears (frontend-design §9; the mock of record is
// design/watch-player). Drawn over the picture, back to front: a bottom scrim carrying the lower
// third, the preview playbar and the transport; then the transient layer, one right-hand sheet over a
// dimmed picture. Presentational: the app owns the player, the data and the input mapping. It is dark
// in a light app too — it sits on a picture, not on a page.

const PlayerChrome = ({
  band,
  density,
  formatTime,
  onPreviewChange,
  playbar,
  sheet,
  transport,
  visible,
}: PlayerChromeProps) => {
  const [previewOpen, setPreviewOpen] = useState(false);
  const preview = (open: boolean) => {
    setPreviewOpen(open);
    onPreviewChange?.(open);
  };
  const touch = density === "touch";
  const shown = visible && !sheet;
  return (
    <ThemeScope theme="dark">
      <View
        aria-hidden={!shown || undefined}
        pointerEvents={shown ? "box-none" : "none"}
        style={{
          bottom: 0,
          gap: touch ? 6 : 12,
          left: 0,
          opacity: shown ? 1 : 0,
          paddingBottom: touch ? 10 : 20,
          paddingHorizontal: touch ? 16 : 32,
          paddingTop: touch ? 40 : 96,
          position: "absolute",
          right: 0,
          zIndex: 30,
          ...gradientOr(
            `linear-gradient(0deg, ${playerChrome.scrimFoot}, ${playerChrome.scrimMid} 55%, transparent)`,
            playerChrome.scrimMid,
          ),
          ...webOnly({ transition: "opacity 220ms ease" }),
        }}
      >
        <LowerThirdBand band={band} density={density} steppedBack={previewOpen && transport.pointer} />
        {playbar ? (
          <PreviewPlaybar
            density={density}
            formatTime={formatTime}
            onPreviewChange={preview}
            playheadMs={playbar.playheadMs}
            pointer={transport.pointer}
            segments={playbar.segments}
            window={playbar.window}
          />
        ) : null}
        <PlayerTransport {...transport} />
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
};

export { PlayerChrome };
