import type { PlayerAudio, PlayerDetails, PlayerOptions, WatchMenuSlot } from "@loomarr/ui";
import type { ComponentProps, RefObject } from "react";
import type { WatchLineupChannel } from "../../watch-lineup";

/** What the open menu is drawn from: the shipped menus' contents, as the player chrome had them. */
interface WatchMenuOptions {
  slot: WatchMenuSlot<WatchLineupChannel>;
  density: "pointer" | "touch";
  audio: ComponentProps<typeof PlayerAudio>["options"];
  switchSound: { on: boolean; onChange: (on: boolean) => void };
  subtitleOptions: ComponentProps<typeof PlayerOptions>["options"];
  onSubtitles: (key: string) => void;
  qualityOptions: ComponentProps<typeof PlayerOptions>["options"] | undefined;
  onQuality: (value: string) => void;
  detailRows: ComponentProps<typeof PlayerDetails>["rows"];
  onCopyDetails: (text: string) => void;
  /** Behind live: Go live, with the live edge's time. */
  goLive: { detail: string; onPress: () => void } | undefined;
  /** The programme on air began inside the window: back to its start. */
  startOver: { detail: string; onPress: () => void } | undefined;
  /** Tunes the channel Last channel names. */
  onLastChannel: (channelId: string) => void;
  video: RefObject<HTMLVideoElement | null>;
}

export type { WatchMenuOptions };
