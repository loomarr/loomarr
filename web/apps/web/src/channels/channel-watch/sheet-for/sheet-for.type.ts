import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { LivePlaybackState } from "@loomarr/player";
import type {
  PlayerAudio,
  PlayerDetails,
  PlayerInfo,
  PlayerOptions,
  PlayerSheet,
  playbarFromTimeline,
} from "@loomarr/ui";
import type { ComponentProps, RefObject } from "react";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import type { WatchChannels } from "../../use-watch-channels";
import type { WatchPlayer } from "../../use-watch-player";
import type { WatchSheet } from "../../use-watch-shortcuts";

type PlaybarData = ReturnType<typeof playbarFromTimeline>;

/** Everything the one open sheet or menu is drawn from. */
type SheetForOptions = {
  audio: ComponentProps<typeof PlayerAudio>["options"];
  chrome: VideoPlayerChrome;
  closeSheet: () => void;
  copyDetails: (text: string) => void;
  density: ComponentProps<typeof PlayerSheet>["density"];
  detailRows: ComponentProps<typeof PlayerDetails>["rows"];
  drawer: WatchChannels["drawer"] | undefined;
  /** Whether the channel's schedule has anything on it: a phone's More keeps the schedule. */
  hasAirings: boolean;
  info: ComponentProps<typeof PlayerInfo>["info"] | undefined;
  live: LivePlaybackState;
  osdChannel: ChannelDTO;
  /** Where a key's sheet goes: a menu on a pointer, a bottom sheet upright, the right-hand sheet on a phone's side. */
  placement: "bottom" | "menu" | "right";
  player: Pick<WatchPlayer, "setQuality" | "tune">;
  qualityOptions: ComponentProps<typeof PlayerOptions>["options"] | undefined;
  segments: PlaybarData["segments"];
  serverNowMs: number | undefined;
  setDetailsOpen: (open: boolean) => void;
  setSubtitlesKey: ComponentProps<typeof PlayerOptions>["onSelect"];
  setSwitchSoundOn: (on: boolean) => void;
  sheet: WatchSheet | undefined;
  startOverMs: number | undefined;
  subtitleOptions: ComponentProps<typeof PlayerOptions>["options"];
  switchSoundOn: boolean;
  toggleSheet: (sheet: WatchSheet) => void;
  touch: boolean;
  video: RefObject<HTMLVideoElement | null>;
  viewerTimeMs: LivePlaybackState["viewerTimeMs"];
  /** The span the schedule draws, from the same read as `segments`. */
  playbackWindow: PlaybarData["window"];
};

export type { SheetForOptions };
