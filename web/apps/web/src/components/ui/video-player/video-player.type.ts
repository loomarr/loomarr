interface VideoPlayerProps {
  // What to play. Any URL the browser can source a <video> from — this primitive knows nothing
  // about where it came from, which is what lets it serve clips, channel streams and previews
  // alike. Ignored when `attach` is provided (that callback owns the source instead).
  src?: string;
  // Rendered over the top-right of the frame, on a scrim. Optional: a player embedded under a
  // heading that already names the thing does not need to repeat it.
  title?: string;
  // Start playing as soon as the source is ready. Safe only when a user gesture just happened —
  // browsers reject autoplay otherwise, which this handles rather than assuming (see the
  // component's play() rejection path).
  autoPlay?: boolean;
  // Rendered in the frame's top-LEFT, opposite the title. The player itself has no opinion about
  // dismissal; a dialog passes its close button through here so the control sits over the video
  // rather than above it.
  leading?: React.ReactNode;
  // LIVE mode (§9.1 V47). A live channel has nothing to seek to, so there is no seek bar; instead
  // the player renders a LiveIndicator (top-left) and, in the control bar, the `scrubber` slot (the
  // mini-guide timeline) as its own full-width row above the buttons — the mock's layout. Play,
  // volume and fullscreen stay.
  live?: boolean;
  // The live scrubber SLOT (§9.1 V47). A live channel cannot seek but has a schedule, so the caller
  // (channel-watch) passes a mini-guide timeline (TimelineScrubber) here and the player renders it
  // FULL-WIDTH in its own row of the control bar. The player stays ignorant of what the strip is (it
  // takes a node, not a channel). Only used in `live` mode.
  scrubber?: React.ReactNode;
  // The top-bar SLOT (live mode) — channel-specific chrome the player itself must not know: the mock
  // shows "CH 1" (left, after the LIVE badge) and the encoder line "h264 · 1080p" (right).
  // channel-watch fills it; the player only places it, keeping "knows nothing about channels".
  topBar?: React.ReactNode;
  // The time-left SLOT (live mode) — the "22m left" the mock shows in the controls row between the
  // volume and fullscreen. It is PROGRAMME time (from the schedule), which the player has no source
  // for; channel-watch derives it from the airings and passes the text/node. Placed by the player.
  timeLeft?: React.ReactNode;
  // Extra bar controls the caller adds just before fullscreen (§9.1 V47) — channel-watch passes the
  // Audio + Subtitle TrackSelectMenus here. They are channel/policy concerns the player must not
  // know, so it only PLACES them (right side of the controls row, left of the fullscreen button).
  barControls?: React.ReactNode;
  // A centered SLOT over the video, beneath the control scrims (§9.1 Watch). The caller renders a
  // transient state here — channel-watch passes the TunerLoader while the stream warms up, so the
  // cold-start beat reads as a tuner acquiring signal instead of a black frame. The player only
  // places it (fills the frame, centered) and stays ignorant of what it is; pass `undefined` to
  // show nothing. It sits UNDER the top/bottom control bars so those stay operable over it.
  overlay?: React.ReactNode;
  // Custom source binding. When provided, the primitive does NOT set `<video src>`; instead it
  // calls `attach(videoEl)` once the element is mounted and invokes the returned cleanup on
  // unmount/source-change. This is the seam the channel-watch surface uses to bind hls.js (which
  // needs `attachMedia`, not a plain `src`) while keeping every accessible control here — the
  // primitive stays clip-and-transport-agnostic, exactly as its header promises.
  attach?: (video: HTMLVideoElement) => () => void;
  className?: string;
}

export type { VideoPlayerProps };
