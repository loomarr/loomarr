import { formatGuideTime } from "@loomarr/core/guide";
import { Stack, Surface, Text } from "@loomarr/design-system";
import { PlayerAudio, PlayerDetails, PlayerMenu, PlayerMore, PlayerOptions } from "@loomarr/ui";
import type { ReactNode } from "react";
import type { WatchMenuOptions } from "./watch-menu.type";

// Picture in picture is the browser's: offered only where it is.
const pipSupported = typeof document !== "undefined" && document.pictureInPictureEnabled === true;

// The shortcuts sheet (`?`): the watching screen's keys on the web, as playback-clients.md lists them.
const SHORTCUTS: readonly (readonly [string, string])[] = [
  ["↑ ↓", "Change channel"],
  ["0-9", "Tune a channel by number"],
  ["Space", "Pause or play"],
  ["End", "Go live"],
  ["← →", "Back or on ten seconds"],
  ["/", "Find a channel"],
  ["G", "Guide"],
  ["L", "Last channel"],
  ["C", "Subtitles"],
  ["A", "Audio"],
  ["M", "Mute"],
  ["F", "Full screen"],
  ["Esc", "Back one step"],
];

/**
 * The open menu in the watching screen's slot: the shipped Subtitles, Audio and More, with Picture
 * and Playback details as More's inner pages, and the shortcuts sheet. A choice leaves its menu
 * open, so the viewer sees the change as they choose.
 */
const watchMenu = ({
  audio,
  density,
  detailRows,
  goLive,
  onCopyDetails,
  onLastChannel,
  onQuality,
  onSubtitles,
  qualityOptions,
  slot,
  startOver,
  subtitleOptions,
  switchSound,
  video,
}: WatchMenuOptions): ReactNode => {
  const { close, lastChannel, name, openPage, page } = slot;
  const menu = (title: string, body: ReactNode, back?: () => void) => (
    // More names no heading: its rows say what they are.
    <PlayerMenu density={density} heading={title !== "More"} key={title} title={title} {...(back ? { onBack: back } : {})}>
      {body}
    </PlayerMenu>
  );
  const toMore = () => openPage(undefined);

  if (name === "subtitles") {
    return menu(
      "Subtitles",
      <PlayerOptions density={density} label="Subtitles" onSelect={onSubtitles} options={subtitleOptions} />,
    );
  }
  if (name === "audio") return menu("Audio", <PlayerAudio density={density} options={audio} switchSound={switchSound} />);
  if (name === "shortcuts") {
    return menu(
      "Keyboard shortcuts",
      <Stack gap="2" paddingHorizontal={10} paddingVertical={6} render="ul">
        {SHORTCUTS.map(([keys, what]) => (
          <Stack align="center" direction="row" gap="2" key={keys} render="li">
            <Surface backgroundColor="transparent" borderWidth={0} width={56}>
              <Text textRole="label" tone="primary">
                {keys}
              </Text>
            </Surface>
            <Text textRole="label" tone="secondary">
              {what}
            </Text>
          </Stack>
        ))}
      </Stack>,
    );
  }
  if (page === "picture" && qualityOptions) {
    return menu(
      "Picture",
      <PlayerOptions density={density} label="Picture quality" onSelect={onQuality} options={qualityOptions} />,
      toMore,
    );
  }
  if (page === "details") {
    return menu("Playback details", <PlayerDetails density={density} onCopy={onCopyDetails} rows={detailRows} />, toMore);
  }
  const last = lastChannel?.channel;
  return menu(
    "More",
    <PlayerMore
      density={density}
      formatTime={formatGuideTime}
      goLive={goLive}
      lastChannel={
        last && lastChannel
          ? {
              detail: `${last.number} ${last.name}`,
              onPress: () => {
                close();
                onLastChannel(last.id);
              },
              ...(lastChannel.playable ? {} : { unavailable: true }),
            }
          : undefined
      }
      picture={
        qualityOptions
          ? {
              detail: qualityOptions.find((option) => option.selected)?.label ?? "Auto",
              onPress: () => openPage("picture"),
            }
          : undefined
      }
      pictureInPicture={
        pipSupported
          ? {
              onPress: () => {
                close();
                void video.current?.requestPictureInPicture().catch(() => undefined);
              },
            }
          : undefined
      }
      playbackDetails={{ onPress: () => openPage("details") }}
      startOver={startOver}
    />,
  );
};

export { watchMenu };
