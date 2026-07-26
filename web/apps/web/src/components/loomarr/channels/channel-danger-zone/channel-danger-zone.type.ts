interface ChannelDangerZoneProps {
  channelName: string;
  // Raw ChannelDTOStatus wire value ("building" | "live" | "drifted" | "detached" |
  // "paused") — only "paused" changes this component's rendering, so it takes the plain
  // string rather than importing the generated enum for one comparison.
  status: string;
  onPause: () => void;
  onResume: () => void;
  // `purge` mirrors the "Also remove it from Tunarr" checkbox — true deletes the Tunarr
  // channel too, false leaves Loomarr's record gone but the Tunarr channel intact.
  onDelete: (opts: { purge: boolean }) => void;
  // True while a pause/resume/delete request from THIS component is in flight, so its
  // controls disable without the parent needing its own local busy state.
  busy?: boolean;
  className?: string;
}

export type { ChannelDangerZoneProps };
