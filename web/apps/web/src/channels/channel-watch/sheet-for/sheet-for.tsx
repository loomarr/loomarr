import { formatGuideTime } from "@loomarr/core/guide";
import { Stack } from "@loomarr/design-system";
import {
  PlayerAudio,
  PlayerBottomSheet,
  PlayerDetails,
  PlayerInfo,
  PlayerMenu,
  PlayerMore,
  PlayerOptions,
  PlayerSheet,
} from "@loomarr/ui";
import type { ReactNode } from "react";
import { ChannelDrawer } from "@/channels/channel-drawer";
import type { SheetForOptions } from "./sheet-for.type";

// Picture in picture is the browser's: offered only where it is.
const pipSupported = typeof document !== "undefined" && document.pictureInPictureEnabled === true;

// Where what a key opens goes (§9.5, variant J): a menu in the control cluster on a pointer, a sheet
// from the bottom on a phone held upright, the right-hand sheet on a phone on its side. Channels is
// still the console drawer (#1659 W1) everywhere, until it is rebuilt on the sheet.
//
// What is open: a menu or a sheet, by placement. Info on a pointer is neither: it is the block on
// air's card on the film strip, pinned open in full (the strip's `info`). Nothing open is undefined,
// which the chrome reads as having no menu or sheet to draw.
const sheetFor = ({
  audio,
  chrome,
  closeSheet,
  copyDetails,
  density,
  detailRows,
  drawer,
  hasAirings,
  info,
  live,
  osdChannel,
  placement,
  player,
  qualityOptions,
  segments,
  serverNowMs,
  setDetailsOpen,
  setSubtitlesKey,
  setSwitchSoundOn,
  sheet,
  startOverMs,
  subtitleOptions,
  switchSoundOn,
  toggleSheet,
  touch,
  video,
  viewerTimeMs,
  playbackWindow,
}: SheetForOptions): ReactNode => {
  // The right-hand sheet's own name for the channel it is about.
  const sheetChannel = { name: osdChannel.name, number: String(osdChannel.number) };
  const sheetOf = (title: string, body: ReactNode, back?: () => void) =>
    placement === "menu" ? (
      // More names no heading: its rows say what they are. Picture is its second page.
      <PlayerMenu
        density={density}
        heading={title !== "More"}
        key={title}
        title={title}
        {...(back ? { onBack: back } : {})}
      >
        {body}
      </PlayerMenu>
    ) : placement === "bottom" ? (
      <PlayerBottomSheet key={title} onClose={closeSheet} title={title}>
        {body}
      </PlayerBottomSheet>
    ) : (
      <PlayerSheet channel={sheetChannel} density={density} key={title} onClose={closeSheet} title={title}>
        {body}
      </PlayerSheet>
    );

  return sheet === "subtitles" ? (
    // A choice leaves the menu open, so the viewer sees the lines change as they choose.
    sheetOf(
      "Subtitles",
      <PlayerOptions
        density={density}
        label="Subtitles"
        onSelect={setSubtitlesKey}
        options={subtitleOptions}
      />,
    )
  ) : sheet === "audio" ? (
    sheetOf(
      "Audio",
      <PlayerAudio
        density={density}
        options={audio}
        switchSound={{ on: switchSoundOn, onChange: setSwitchSoundOn }}
      />,
    )
  ) : sheet === "picture" ? (
    sheetOf(
      "Picture",
      qualityOptions ? (
        <PlayerOptions
          density={density}
          label="Picture quality"
          onSelect={(value) => player.setQuality(value === "auto" ? "auto" : Number(value))}
          options={qualityOptions}
        />
      ) : null,
      placement === "menu" ? () => toggleSheet("more") : undefined,
    )
  ) : sheet === "more" ? (
    sheetOf(
      "More",
      <PlayerMore
        channels={drawer ? { onPress: () => toggleSheet("channels") } : undefined}
        density={density}
        formatTime={formatGuideTime}
        goLive={
          live.mode === "live"
            ? undefined
            : { detail: formatGuideTime(serverNowMs ?? Date.now()), onPress: chrome.goLive }
        }
        info={info ? { onPress: () => toggleSheet("info") } : undefined}
        pictureInPicture={
          pipSupported
            ? {
                onPress: () => {
                  closeSheet();
                  void video.current?.requestPictureInPicture().catch(() => undefined);
                },
              }
            : undefined
        }
        playbackDetails={{
          onPress: () => {
            if (touch) toggleSheet("details");
            else {
              closeSheet();
              setDetailsOpen(true);
            }
          },
        }}
        picture={
          qualityOptions
            ? {
                detail: qualityOptions.find((o) => o.selected)?.label ?? "Auto",
                onPress: () => toggleSheet("picture"),
              }
            : undefined
        }
        // A phone has no film strip in its band, so its More keeps the schedule.
        schedule={
          touch && hasAirings
            ? {
                onSeek: chrome.seekTo,
                playheadMs: viewerTimeMs,
                segments,
                window: playbackWindow,
              }
            : undefined
        }
        startOver={
          startOverMs === undefined || !info
            ? undefined
            : { detail: info.title, onPress: () => chrome.seekTo(startOverMs) }
        }
      />,
    )
  ) : sheet === "details" ? (
    sheetOf("Playback details", <PlayerDetails density={density} onCopy={copyDetails} rows={detailRows} />)
  ) : sheet === "info" && info && placement !== "menu" ? (
    sheetOf("Info", <PlayerInfo density={density} formatTime={formatGuideTime} info={info} />)
  ) : sheet === "channels" && drawer ? (
    <Stack className="absolute inset-y-0 right-0 z-40 max-w-full" direction="row">
      <ChannelDrawer
        channels={drawer.channels}
        tunedId={osdChannel.id}
        favourites={drawer.favourites}
        recent={drawer.recent}
        nowPercent={drawer.nowPercent}
        onTune={player.tune}
        onFavourite={drawer.onFavourite}
        onClose={closeSheet}
      />
    </Stack>
  ) : undefined;
};

export { sheetFor };
