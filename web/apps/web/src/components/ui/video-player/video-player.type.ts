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
  // LIVE mode. A live channel has nothing to seek to (§9.1), so the scrubber is replaced by a LIVE
  // indicator and keyboard scrubbing is disabled — presenting a seek bar that does nothing would
  // be a control that lies. Play/pause and volume stay, because leaving and rejoining a live
  // stream is still meaningful.
  live?: boolean;
  // Custom source binding. When provided, the primitive does NOT set `<video src>`; instead it
  // calls `attach(videoEl)` once the element is mounted and invokes the returned cleanup on
  // unmount/source-change. This is the seam the channel-watch surface uses to bind hls.js (which
  // needs `attachMedia`, not a plain `src`) while keeping every accessible control here — the
  // primitive stays clip-and-transport-agnostic, exactly as its header promises.
  attach?: (video: HTMLVideoElement) => () => void;
  className?: string;
}

export type { VideoPlayerProps };
