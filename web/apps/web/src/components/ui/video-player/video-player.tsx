import { useEffect, useLayoutEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import { FullscreenButton } from "./fullscreen-button";
import { HoldControlsContext } from "./internal/hold-controls-context";
import { useAutoHideControls } from "./internal/use-auto-hide-controls";
import { useFullscreen } from "./internal/use-fullscreen";
import { usePictureInPicture } from "./internal/use-picture-in-picture";
import { usePlaybackState } from "./internal/use-playback-state";
import { PlayToggle } from "./play-toggle";
import type { VideoPlayerProps } from "./video-player.type";
import { VolumeControl } from "./volume-control";

// VideoPlayer — the app's own video surface, with Loomarr's controls rather than the browser's
// (V39, frontend-design §3 Layer 1). A clip gets its named controls (PlayToggle, VolumeControl,
// FullscreenButton); a live channel gets the caller's §9 player chrome through `renderChrome`.
//
// ⚠ **Custom controls rather than `<video controls>` — a maintainer decision (2026-08-03).** Native
// controls are keyboard-correct and free; hand-built ones make that this file's job (see the
// keyboard shortcuts). What they buy is a player that reads as part of the app.
//
// ⚠ **Knows NOTHING about clips or channels.** It takes a `src`/`attach` and slots (`overlay`,
// `renderChrome`); the filler catalog hands it a clip URL, channel-watch hands it hls.js and the
// player chrome. Putting `ClipDTO`/channel concepts in here would make "core primitive" a lie.

// mmss renders a media time (m:ss) — for the non-live clip player's progress readout.
const mmss = (seconds: number): string => {
  const total = Math.max(0, Math.floor(seconds || 0));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

// The scrub step for arrow keys, in seconds (non-live only). Five is the convention.
const SCRUB_STEP = 5;
// A live channel's ⟲/⟳ and ←/→ step (#2088 decision 2).
const LIVE_SEEK_STEP_MS = 10_000;
// −/= step the volume a tenth at a time, as the slider's arrow keys do.
const VOLUME_STEP = 0.1;
// Two taps this close together on the same side of a touch frame are a double-tap.
const DOUBLE_TAP_MS = 300;

// Hold the outgoing decoded picture inside the same <video> while an attached source is replaced.
// This is a poster, not a hidden player or decoder: transport teardown can release immediately,
// while the viewer keeps the last honest frame until the replacement produces its first one.
const holdDecodedFrame = (video: HTMLVideoElement) => {
  if (
    video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA ||
    video.videoWidth < 1 ||
    video.videoHeight < 1
  ) {
    return;
  }
  const canvas = document.createElement("canvas");
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  const context = canvas.getContext("2d");
  if (!context) return;
  try {
    context.drawImage(video, 0, 0, canvas.width, canvas.height);
    video.poster = canvas.toDataURL("image/png");
  } catch {
    // A cross-origin source without canvas permission cannot be captured. Leave the element's
    // existing poster alone; source replacement still proceeds normally.
  }
};

const clearHeldFrame = (video: HTMLVideoElement) => {
  if (video.poster.startsWith("data:image/png;base64,")) video.removeAttribute("poster");
};

const VideoPlayer = ({
  src,
  title,
  autoPlay,
  onPlaybackStart,
  startAt,
  endAt,
  leading,
  live,
  liveTransport,
  renderChrome,
  overlay,
  onShortcut,
  attach,
  onChannelStep,
  className,
}: VideoPlayerProps) => {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);

  // The three concerns, each its own tested hook (see ./internal): the element's transport, the
  // fullscreen quirk, and the auto-hide overlay. VideoPlayer is left composing them into the frame.
  const {
    playing,
    muted,
    setMuted,
    volume,
    setVolume,
    current,
    duration,
    ready,
    toggle,
    seekTo,
    mediaHandlers,
    // ⚠ The window is dropped in LIVE mode: a live duration is Infinity, so there is nothing to
    // clamp and a stray `endAt` would pause the stream at a number that means nothing.
  } = usePlaybackState(videoRef, live ? {} : { startAt, endAt }, liveTransport);
  const { fullscreen, toggleFullscreen } = useFullscreen(wrapperRef);
  const { pictureInPicture, pictureInPictureSupported, togglePictureInPicture } =
    usePictureInPicture(videoRef);
  const { controlsShown, hideControls, holdControls, onPointerActive, onPointerLeave, revealControls } =
    useAutoHideControls(playing);

  // Custom source binding (hls.js). `attach(el)` runs in the commit before WebKit can defer passive
  // effects behind outgoing media work; the Tuner has already painted its acknowledgement before
  // changing this source. Stays here because it wires `attach` onto the element this component owns.
  useLayoutEffect(() => {
    if (!attach) return;
    const el = videoRef.current;
    if (!el) return;
    const release = attach(el);
    const onReplacementFrame = () => clearHeldFrame(el);
    // Registering rVFC immediately can observe the outgoing decoded frame that cleanup just
    // captured, while registering it after loadeddata can miss a compact replacement's only
    // presented frame. loadeddata itself means the replacement has current frame data, which is
    // exactly when the held poster should give way.
    const onReplacementLoadedData = onReplacementFrame;
    el.addEventListener("loadeddata", onReplacementLoadedData, { once: true });
    return () => {
      el.removeEventListener("loadeddata", onReplacementLoadedData);
      holdDecodedFrame(el);
      release();
    };
  }, [attach]);

  // A live player with a tuner seam owns keyboard focus once, on mount. The shortcuts below are bound
  // to the wrapper, and a freshly tuned page has focus on <body>, so without this the first channel
  // key after tuning in went nowhere (#1459). Mount-only: later focus belongs to the viewer.
  const tunable = Boolean(live && onChannelStep);
  // biome-ignore lint/correctness/useExhaustiveDependencies: mount-only claim; re-running on `tunable` flips must not steal focus back
  useLayoutEffect(() => {
    if (tunable && document.activeElement === document.body)
      wrapperRef.current?.focus({ preventScroll: true });
  }, []);

  // A live channel's window (#2088 decision 2): ←/→ and the chrome's ⟲/⟳ move 10 s through the
  // fifteen minutes the stream holds, Start over and the playbar's past name an instant, and L goes
  // live. Forward from live has nowhere to go. All of it acts on the player's own element through the
  // caller's live transport; without one (or without its `seek`) they do nothing.
  const seekLiveTo = (wallClockMs: number) => {
    const el = videoRef.current;
    if (el && liveTransport?.seek) void liveTransport.seek(el, wallClockMs);
  };
  const seekLiveBy = (deltaMs: number) => {
    if (!liveTransport || (deltaMs > 0 && liveTransport.state.mode === "live")) return;
    seekLiveTo(liveTransport.state.viewerTimeMs + deltaMs);
  };
  const goLive = () => {
    const el = videoRef.current;
    if (el && liveTransport) void liveTransport.goLive(el);
  };

  // Touch (#2088 decision 12): a tap shows or hides the controls rather than pausing, and on a live
  // channel a double-tap on the left or right third steps 10 s. A tap waits out the double-tap window
  // before acting, so the second tap of a double is never also a toggle. The last pointer's kind is
  // kept because a touch also fires the mouse events a mouse would, which must not reveal the controls
  // on their own.
  const pointerKindRef = useRef("mouse");
  const notePointer = (e: React.PointerEvent) => {
    pointerKindRef.current = e.pointerType;
  };
  const tapRef = useRef<{ at: number; side: number; timer?: number }>({ at: 0, side: 0 });
  useEffect(() => () => window.clearTimeout(tapRef.current.timer), []);
  const onFrameClick = (e: React.MouseEvent<HTMLVideoElement>) => {
    if (pointerKindRef.current !== "touch") {
      toggle();
      return;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    const side = x < 1 / 3 ? -1 : x > 2 / 3 ? 1 : 0;
    const now = performance.now();
    const tap = tapRef.current;
    window.clearTimeout(tap.timer);
    if (live && side !== 0 && side === tap.side && now - tap.at < DOUBLE_TAP_MS) {
      tapRef.current = { at: 0, side: 0 };
      seekLiveBy(side * LIVE_SEEK_STEP_MS);
      return;
    }
    const shown = controlsShown;
    tapRef.current = {
      at: now,
      side,
      timer: window.setTimeout(() => (shown ? hideControls() : revealControls()), DOUBLE_TAP_MS),
    };
  };
  const onMouseActive = () => {
    if (pointerKindRef.current !== "touch") onPointerActive();
  };
  const onMouseGone = () => {
    if (pointerKindRef.current !== "touch") onPointerLeave();
  };

  // Keyboard shortcuts — what native controls would have given free: Space/K toggle, M mutes, −/=
  // step the volume, F fullscreen, P picture-in-picture, arrows seek (a live channel within its
  // window), L goes live. Bound on the player's surface so they
  // cannot fire while it is off-screen, and skipped when focus is in a control that uses the same
  // keys (Space on a <button> must click).
  const onKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    // Typing belongs to the field: a search box in the panel must be able to take "m" and "k".
    if (target.closest("input:not([type=range]), textarea, select")) return;
    const isSlider = target.getAttribute("role") === "slider";
    // The live chrome's controls are RN-web Pressables — `div[role=button]`, not <button> — and
    // they act on Space and Enter themselves, so the frame must not act on the key a second time.
    const isButton = Boolean(target.closest("button, [role=button]"));
    const ownsNavigationKey = Boolean(
      target.closest(
        "button, [role=button], input, select, textarea, [role=slider], [role=menuitem], [role=menuitemcheckbox]",
      ),
    );
    switch (e.key) {
      case " ":
      case "k":
        if (isButton) return;
        e.preventDefault();
        toggle();
        break;
      case "ArrowLeft":
        if (isSlider) return;
        e.preventDefault();
        if (live) seekLiveBy(-LIVE_SEEK_STEP_MS);
        else seekTo(current - SCRUB_STEP);
        break;
      case "ArrowRight":
        if (isSlider) return;
        e.preventDefault();
        if (live) seekLiveBy(LIVE_SEEK_STEP_MS);
        else seekTo(current + SCRUB_STEP);
        break;
      case "l":
      case "L":
        if (!live || !liveTransport) return;
        e.preventDefault();
        goLive();
        break;
      case "m":
      case "M":
        e.preventDefault();
        setMuted((m) => !m);
        break;
      case "-":
      case "=":
      case "+":
        if (isSlider) return;
        e.preventDefault();
        setVolume(Math.min(1, Math.max(0, volume + (e.key === "-" ? -VOLUME_STEP : VOLUME_STEP))));
        break;
      case "f":
      case "F":
        e.preventDefault();
        toggleFullscreen();
        break;
      case "p":
      case "P":
        if (!pictureInPictureSupported) return;
        e.preventDefault();
        togglePictureInPicture();
        break;
      case "ArrowUp":
      case "PageUp":
      case "ChannelUp":
        if (!live || !onChannelStep || ownsNavigationKey) return;
        e.preventDefault();
        onChannelStep(1);
        break;
      case "ArrowDown":
      case "PageDown":
      case "ChannelDown":
        if (!live || !onChannelStep || ownsNavigationKey) return;
        e.preventDefault();
        onChannelStep(-1);
        break;
      default:
        // A key the player doesn't use goes to the caller, unless focus is typing or in a menu.
        if (!onShortcut || e.metaKey || e.ctrlKey || e.altKey) return;
        if (target.closest("[role=menuitem], [role=menuitemcheckbox]")) return;
        if (onShortcut(e.key)) e.preventDefault();
        break;
    }
  };

  return (
    <HoldControlsContext.Provider value={holdControls}>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: shortcut layer over a composite widget; every action also has a focusable control inside */}
      <div
        ref={wrapperRef}
        className={cn(
          "group/player relative aspect-video w-full overflow-hidden bg-black outline-none",
          className,
        )}
        // Focusable by script only (not a Tab stop) so the mount claim above can land on the surface.
        tabIndex={-1}
        onKeyDown={onKeyDown}
        onPointerDown={notePointer}
        onPointerMove={notePointer}
        onMouseEnter={onMouseActive}
        onMouseMove={onMouseActive}
        // Leaving just drops `hovering`; the effect arms the short GRACE hide. A menu open at the time
        // still keeps the bar (held > 0), so leaving TOWARD the portalled menu doesn't pull it away.
        onMouseLeave={onMouseGone}
        onFocus={revealControls}
      >
        <video
          ref={videoRef}
          // `attach` owns the source when present (hls.js binds via attachMedia); else the plain URL.
          src={attach ? undefined : src}
          className={cn("size-full cursor-pointer", !controlsShown && "cursor-none")}
          autoPlay={autoPlay}
          muted={muted}
          playsInline
          onClick={onFrameClick}
          onPlaying={onPlaybackStart}
          // play/pause, elapsed, and metadata-load are owned by usePlaybackState.
          {...mediaHandlers}
        />

        {/* OVERLAY SLOT — a caller-rendered transient state (channel-watch's TunerLoader during
          warm-up). Sits directly over the video but UNDER the control-bar scrims below, so the
          controls stay operable. pointer-events-none so it never eats a click on the frame. */}
        {overlay && <div className="pointer-events-none absolute inset-0 z-0">{overlay}</div>}

        {/* LIVE: the caller's chrome (§9), drawn from the player's own state. */}
        {live &&
          renderChrome?.({
            controlsShown,
            fullscreen,
            goLive,
            hide: hideControls,
            hold: holdControls.hold,
            muted,
            pictureInPicture,
            playing,
            seekBy: seekLiveBy,
            seekTo: seekLiveTo,
            setMuted,
            setVolume,
            toggle,
            toggleFullscreen,
            togglePictureInPicture: pictureInPictureSupported ? togglePictureInPicture : undefined,
            volume,
          })}

        {/* TOP BAR (a clip) — `leading` (left) + `title` (right), over a scrim, fading with the
          controls. */}
        {!live && (title || leading) && (
          <div
            className={cn(
              "pointer-events-none absolute top-0 right-0 left-0 flex items-center gap-2.5 bg-linear-to-b from-black/70 to-transparent p-3 transition-opacity duration-200",
              controlsShown ? "opacity-100" : "opacity-0",
            )}
          >
            <div className="pointer-events-auto shrink-0">{leading}</div>
            {title && (
              <p className="text-static-100 pointer-events-auto ml-auto min-w-0 truncate text-right text-sm font-medium">
                {title}
              </p>
            )}
          </div>
        )}

        {/* CONTROL BAR (a clip) — over a bottom scrim, auto-hiding: play, volume and the elapsed time
          left-packed, fullscreen at the far edge. */}
        {!live && (
          <div
            className={cn(
              "absolute right-0 bottom-0 left-0 flex flex-col gap-2.5 bg-linear-to-t from-black/80 via-black/40 to-transparent px-4 pt-8 pb-3 transition-opacity duration-200",
              controlsShown ? "opacity-100" : "pointer-events-none opacity-0",
            )}
          >
            <fieldset className="flex min-w-0 items-center gap-2 border-0 p-0 sm:gap-3">
              <legend className="sr-only">Playback controls</legend>
              <PlayToggle playing={playing} onToggle={toggle} />
              <VolumeControl
                volume={volume}
                muted={muted}
                onVolumeChange={setVolume}
                onMutedChange={setMuted}
              />
              {ready && (
                <span className="text-static-300 shrink-0 font-mono text-xs tabular-nums">
                  {mmss(current)} <span className="text-muted-foreground">/ {mmss(duration)}</span>
                </span>
              )}
              <div className="ml-auto flex items-center gap-1.5">
                <FullscreenButton active={fullscreen} onToggle={toggleFullscreen} />
              </div>
            </fieldset>
          </div>
        )}
      </div>
    </HoldControlsContext.Provider>
  );
};

export { VideoPlayer };
