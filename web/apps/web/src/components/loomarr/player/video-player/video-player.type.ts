import type { LivePlaybackTransport } from "./live-playback-transport.type";

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
  // Called from the media element's `playing` event, after playback has actually started or
  // resumed. This is deliberately stronger than a play-button click or `play()` request: callers
  // such as filler review use it to prove that the exact resolved bytes reached playback before
  // enabling a semantic answer.
  onPlaybackStart?: () => void;
  // Play only a WINDOW of the source (§10 V54), in SECONDS of media time — the unit the element
  // and `current`/`duration`/`seekTo` already speak, so a caller holding milliseconds converts.
  //
  // Built for the split-review gate: a proposed cut has no bytes of its own until it is confirmed,
  // so previewing one means playing a slice of the 20-minute composite it was detected in.
  //
  // ⚠ **With a window set, `duration` is the WINDOW's length and `current` is measured from
  // `startAt`** — a 30-second cut of a 22-minute reel reads "0:04 / 0:30". That is the entire
  // point: handing the readout the reel's numbers would present the whole recording as if it were
  // the clip, which is what makes a preview useless for judging one cut.
  //
  // ⚠ Ignored in `live` mode — a live duration is Infinity and there is nothing to clamp.
  startAt?: number;
  endAt?: number;
  // Rendered in the frame's top-LEFT, opposite the title. The player itself has no opinion about
  // dismissal; a dialog passes its close button through here so the control sits over the video
  // rather than above it.
  leading?: React.ReactNode;
  // LIVE mode. A live channel has nothing to seek to and a schedule the player knows nothing about,
  // so the player draws no controls of its own: the caller's `renderChrome` draws them (the §9
  // player chrome, frontend-design.md), over the picture and inside the frame, so they go fullscreen
  // with it. Without `renderChrome` the picture is bare (a portrait phone's controls sit under it).
  live?: boolean;
  // Optional live-clock controller. Channel Watch supplies it so the play toggle preserves an
  // intentional paused point and the chrome can offer a one-action return to live.
  liveTransport?: LivePlaybackTransport;
  // Live mode's controls, drawn by the caller from the player's own state (§9). The player keeps
  // what only it can own — the element's transport, fullscreen, the auto-hide and the keys — and
  // hands them over; the caller draws everything. `hold(true)` keeps the controls shown while
  // something in them is open (a preview card, a sheet), as an open menu does.
  renderChrome?: (chrome: VideoPlayerChrome) => React.ReactNode;
  // Custom source binding. When provided, the primitive does NOT set `<video src>`; instead it
  // calls `attach(videoEl)` once the element is mounted and invokes the returned cleanup on
  // unmount/source-change. This is the seam the channel-watch surface uses to bind hls.js (which
  // needs `attachMedia`, not a plain `src`) while keeping every accessible control here — the
  // primitive stays clip-and-transport-agnostic, exactly as its header promises.
  attach?: (video: HTMLVideoElement) => () => void;
  // Fill the container's height instead of keeping a 16:9 frame; the <video> letterboxes the
  // picture inside it, centred. On an upright phone that is the strip mid-screen the chrome's toolbar,
  // band and keys sit clear of (§9.7).
  fill?: boolean;
  // The frame's own keys: Space and K play and pause, M mutes, ←/→ seek, L goes live. On unless a
  // caller handles every key itself (the watching screen, whose keys mean other things), which
  // passes false so no key is handled twice.
  keys?: boolean;
}

// What a live player's caller draws its controls from (`renderChrome`).
interface VideoPlayerChrome {
  // The controls should be showing: the pointer moved, focus is inside, or playback is paused.
  controlsShown: boolean;
  fullscreen: boolean;
  // Back to the live edge (the live transport's Go live).
  goLive: () => void;
  // Hide the controls now, for a moment the caller draws alone (a station-ID card); held or paused
  // controls stay. The next pointer move or focus shows them again.
  hide: () => void;
  hold: (on: boolean) => void;
  muted: boolean;
  playing: boolean;
  // Through the live window (#2088 decision 2): by a step from the frame on screen (forward from
  // live does nothing), or to a wall-clock instant; reaching the edge goes live.
  seekBy: (deltaMs: number) => void;
  seekTo: (wallClockMs: number) => void;
  setMuted: (muted: boolean) => void;
  setVolume: (volume: number) => void;
  toggle: () => void;
  toggleFullscreen: () => void;
  volume: number;
}

export type { VideoPlayerChrome, VideoPlayerProps };
