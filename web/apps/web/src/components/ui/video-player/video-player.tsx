import { useEffect, useRef } from "react";
import { cn } from "@/lib";
import { FullscreenButton } from "./fullscreen-button";
import { HoldControlsContext } from "./internal/hold-controls-context";
import { useAutoHideControls } from "./internal/use-auto-hide-controls";
import { useFullscreen } from "./internal/use-fullscreen";
import { usePlaybackState } from "./internal/use-playback-state";
import { LiveIndicator } from "./live-indicator";
import { PlayToggle } from "./play-toggle";
import type { VideoPlayerProps } from "./video-player.type";
import { VolumeControl } from "./volume-control";

// VideoPlayer — the app's own video surface, with Loomarr's controls rather than the browser's
// (V39, frontend-design §3 Layer 1). It COMPOSES named controls (PlayToggle, VolumeControl,
// FullscreenButton, LiveIndicator, and the live scrubber slot) rather than hand-rolling each.
//
// ⚠ **Custom controls rather than `<video controls>` — a maintainer decision (2026-08-03).** Native
// controls are keyboard-correct and free; hand-built ones make that this file's job (see the
// keyboard shortcuts). What they buy is a player that reads as part of the app.
//
// ⚠ **Knows NOTHING about clips or channels.** It takes a `src`/`attach` and slots (`topBar`,
// `scrubber`); the filler catalog hands it a clip URL, channel-watch hands it hls.js + a live
// timeline. Putting `ClipDTO`/channel concepts in here would make "core primitive" a lie.

// mmss renders a media time (m:ss) — for the non-live clip player's progress readout.
const mmss = (seconds: number): string => {
  const total = Math.max(0, Math.floor(seconds || 0));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

// The scrub step for arrow keys, in seconds (non-live only). Five is the convention.
const SCRUB_STEP = 5;

const VideoPlayer = ({
  src,
  title,
  autoPlay,
  startAt,
  endAt,
  leading,
  live,
  scrubber,
  topBar,
  timeLeft,
  barControls,
  overlay,
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
  } = usePlaybackState(videoRef, live ? {} : { startAt, endAt });
  const { fullscreen, toggleFullscreen } = useFullscreen(wrapperRef);
  const { controlsShown, holdControls, onPointerActive, onPointerLeave, revealControls } =
    useAutoHideControls(playing);

  // Custom source binding (hls.js). `attach(el)` runs on mount and its cleanup on unmount. Stays in
  // the component because it wires the caller's `attach` onto the element the component owns.
  useEffect(() => {
    if (!attach) return;
    const el = videoRef.current;
    if (!el) return;
    return attach(el);
  }, [attach]);

  // Keyboard shortcuts — what native controls would have given free: Space/K toggle, M mutes, arrows
  // seek (non-live only). Bound on the player's surface so they cannot fire while it is off-screen,
  // and skipped when focus is in a control that uses the same keys (Space on a <button> must click).
  const onKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    const isSlider = target.getAttribute("role") === "slider";
    const isButton = target.tagName === "BUTTON";
    const ownsNavigationKey = Boolean(
      target.closest(
        "button, input, select, textarea, [role=slider], [role=menuitem], [role=menuitemcheckbox]",
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
        // Live has nothing to seek to, so the scrub shortcuts are inert.
        if (isSlider || live) return;
        e.preventDefault();
        seekTo(current - SCRUB_STEP);
        break;
      case "ArrowRight":
        if (isSlider || live) return;
        e.preventDefault();
        seekTo(current + SCRUB_STEP);
        break;
      case "m":
        e.preventDefault();
        setMuted((m) => !m);
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
        break;
    }
  };

  return (
    <HoldControlsContext.Provider value={holdControls}>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: shortcut layer over a composite widget; every action also has a focusable control inside */}
      <div
        ref={wrapperRef}
        className={cn("group/player relative aspect-video w-full overflow-hidden bg-black", className)}
        onKeyDown={onKeyDown}
        onMouseEnter={onPointerActive}
        onMouseMove={onPointerActive}
        // Leaving just drops `hovering`; the effect arms the short GRACE hide. A menu open at the time
        // still keeps the bar (held > 0), so leaving TOWARD the portalled menu doesn't pull it away.
        onMouseLeave={onPointerLeave}
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
          onClick={toggle}
          // play/pause, elapsed, and metadata-load are owned by usePlaybackState.
          {...mediaHandlers}
        />

        {/* OVERLAY SLOT — a caller-rendered transient state (channel-watch's TunerLoader during
          warm-up). Sits directly over the video but UNDER the control-bar scrims below, so the
          controls stay operable. pointer-events-none so it never eats a click on the frame. */}
        {overlay && <div className="pointer-events-none absolute inset-0 z-0">{overlay}</div>}

        {/* TOP BAR — over a scrim, fading with the controls. Live: LiveIndicator (left) + the caller's
          `topBar` (CH + encoder line), matching the mock. Non-live: `leading` (left) + `title`
          (right), the clip player's existing chrome. */}
        {(live ? topBar : title || leading) && (
          <div
            className={cn(
              "pointer-events-none absolute top-0 right-0 left-0 flex items-center gap-2.5 bg-linear-to-b from-black/70 to-transparent p-3 transition-opacity duration-200",
              controlsShown ? "opacity-100" : "opacity-0",
            )}
          >
            {live ? (
              <>
                <LiveIndicator />
                <div className="pointer-events-auto flex min-w-0 flex-1 items-center gap-2.5">{topBar}</div>
              </>
            ) : (
              <>
                <div className="pointer-events-auto shrink-0">{leading}</div>
                {title && (
                  <p className="pointer-events-auto ml-auto min-w-0 truncate text-right font-medium text-sm text-static-100">
                    {title}
                  </p>
                )}
              </>
            )}
          </div>
        )}

        {/* CONTROL BAR — over a bottom scrim, auto-hiding. A COLUMN: the live scrubber gets its own
          FULL-WIDTH row above the buttons (the mock); the buttons row follows. */}
        <div
          className={cn(
            "absolute right-0 bottom-0 left-0 flex flex-col gap-2.5 bg-linear-to-t from-black/80 via-black/40 to-transparent px-4 pt-8 pb-3 transition-opacity duration-200",
            controlsShown ? "opacity-100" : "pointer-events-none opacity-0",
          )}
        >
          {/* Row 1 (live): the full-width mini-guide scrubber. */}
          {live && scrubber && <div className="w-full">{scrubber}</div>}

          {/* Row 2 — the mock's control row: LEFT-PACKED play → volume → time, with only fullscreen
            pushed to the far right (ml-auto). No stretching spacer: everything sits next to play. */}
          <div className="flex items-center gap-3">
            <PlayToggle playing={playing} onToggle={toggle} />
            <VolumeControl
              volume={volume}
              muted={muted}
              onVolumeChange={setVolume}
              onMutedChange={setMuted}
            />

            {/* Time, right after volume (mock order). Live: the caller's programme time (schedule);
              non-live (a clip): the video's own elapsed / total. */}
            {live
              ? timeLeft && (
                  <span className="shrink-0 font-mono text-static-300 text-xs tabular-nums">{timeLeft}</span>
                )
              : ready && (
                  <span className="shrink-0 font-mono text-static-300 text-xs tabular-nums">
                    {mmss(current)} <span className="text-muted-foreground">/ {mmss(duration)}</span>
                  </span>
                )}

            {/* The right cluster (ml-auto): the caller's bar controls (audio/subtitles) then fullscreen
              at the far edge — the "controls beside fullscreen" the maintainer asked for. */}
            <div className="ml-auto flex items-center gap-1.5">
              {barControls}
              <FullscreenButton active={fullscreen} onToggle={toggleFullscreen} />
            </div>
          </div>
        </div>
      </div>
    </HoldControlsContext.Provider>
  );
};

export { VideoPlayer };
