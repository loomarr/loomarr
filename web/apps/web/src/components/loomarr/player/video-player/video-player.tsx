import { ClipChrome } from "@loomarr/ui";
import { useEffect, useLayoutEffect, useRef } from "react";
import { HoldControlsContext } from "./internal/hold-controls-context";
import { useAutoHideControls } from "./internal/use-auto-hide-controls";
import { useFullscreen } from "./internal/use-fullscreen";
import { usePlaybackState } from "./internal/use-playback-state";
import type { VideoPlayerProps } from "./video-player.type";

// VideoPlayer — the app's own video surface, with Loomarr's controls rather than the browser's
// (V39). A clip gets the design system's ClipChrome; a live channel gets the caller's §9 player
// chrome through `renderChrome`. This file is the DOM half: the element, its keys, fullscreen and
// the auto-hide. What the controls look like is the chrome's.
//
// ⚠ **Custom controls rather than `<video controls>` — a maintainer decision (2026-08-03).** Native
// controls are keyboard-correct and free; hand-built ones make that this file's job (see the
// keyboard shortcuts). What they buy is a player that reads as part of the app.
//
// ⚠ **Knows NOTHING about clips or channels.** It takes a `src`/`attach` and slots (`overlay`,
// `renderChrome`); the filler catalog hands it a clip URL, channel-watch hands it hls.js and the
// player chrome. Putting `ClipDTO`/channel concepts in here would make "core primitive" a lie.

// The scrub step for arrow keys, in seconds (non-live only). Five is the convention.
const SCRUB_STEP = 5;
// A live channel's ⟲/⟳ and ←/→ step (#2088 decision 2).
const LIVE_SEEK_STEP_MS = 10_000;

// The channel keys and which way each one tunes.
const CHANNEL_STEPS: Readonly<Record<string, 1 | -1>> = {
  ArrowUp: 1,
  PageUp: 1,
  ChannelUp: 1,
  ArrowDown: -1,
  PageDown: -1,
  ChannelDown: -1,
};

// Where focus sits in something that answers ↑/↓ itself: typing and choosing fields (every <input>,
// so a native range or radio too), a slider (the chrome's volume steps on ↑/↓, as ARIA asks),
// menus, lists of options, radio groups, tabs and grids, and anything inside a sheet or dialog
// (their own lists may walk on the arrows). A plain button is NOT here: it does nothing with ↑/↓,
// and the chrome's controls are buttons, so leaving them out is what lets the viewer change channel
// after clicking one.
const OWNS_CHANNEL_KEYS = [
  "input",
  "textarea",
  "select",
  "[contenteditable]:not([contenteditable=false])",
  "[role=slider]",
  "[role=menu]",
  "[role=menuitem]",
  "[role=menuitemcheckbox]",
  "[role=menuitemradio]",
  "[role=listbox]",
  "[role=option]",
  "[role=radiogroup]",
  "[role=radio]",
  "[role=tablist]",
  "[role=tab]",
  "[role=grid]",
  "[role=dialog]",
].join(", ");

// Which way a keydown tunes, or undefined when it is not the player's to take: not a channel key,
// already handled by whatever has focus, a browser chord, focus in a control that owns the arrows,
// or a sheet or dialog open anywhere on the page.
const channelStepFor = (e: KeyboardEvent): 1 | -1 | undefined => {
  const step = CHANNEL_STEPS[e.key];
  if (step === undefined || e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return undefined;
  if (e.target instanceof Element && e.target.closest(OWNS_CHANNEL_KEYS)) return undefined;
  if (document.querySelector("[role=dialog]")) return undefined;
  return step;
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
  fill = false,
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
  const { controlsShown, hideControls, holdControls, onPointerActive, onPointerLeave, revealControls } =
    useAutoHideControls(playing);

  // Custom source binding (hls.js). `attach(el)` runs in the commit before WebKit can defer passive
  // effects behind outgoing media work. Stays here because it wires `attach` onto the element this
  // component owns; what plays on it, and what the viewer sees while a source is replaced, is the
  // attached transport's.
  useLayoutEffect(() => {
    if (!attach) return;
    const el = videoRef.current;
    if (!el) return;
    return attach(el);
  }, [attach]);

  // A live player with a tuner seam owns keyboard focus once, on mount. The frame's shortcuts below
  // are bound to the wrapper, and a freshly tuned page has focus on <body>, so without this the first
  // key after tuning in (Space, M, ←/→…) went nowhere (#1459). Mount-only: later focus belongs to the
  // viewer.
  const tunable = Boolean(live && onChannelStep);
  // biome-ignore lint/correctness/useExhaustiveDependencies: mount-only claim; re-running on `tunable` flips must not steal focus back
  useLayoutEffect(() => {
    if (tunable && document.activeElement === document.body)
      wrapperRef.current?.focus({ preventScroll: true });
  }, []);

  // The channel keys are the page's, not the frame's: while a live, tunable player is up, ↑/↓,
  // PgUp/PgDn and ChannelUp/Down change channel wherever focus is, on a chrome button, on <body>
  // after a click outside, or on the frame itself. Bound to the document for the player's lifetime, so
  // focus leaving the frame no longer loses them. This is the ONLY place they are handled; the frame's
  // handler lets them pass, so one press is one step. `channelStepFor` keeps them away from controls
  // that use the arrows themselves.
  const channelStepRef = useRef(onChannelStep);
  useEffect(() => {
    channelStepRef.current = onChannelStep;
  });
  useEffect(() => {
    if (!tunable) return;
    const onDocumentKeyDown = (e: KeyboardEvent) => {
      const step = channelStepFor(e);
      if (step === undefined) return;
      e.preventDefault();
      channelStepRef.current?.(step);
    };
    document.addEventListener("keydown", onDocumentKeyDown);
    return () => document.removeEventListener("keydown", onDocumentKeyDown);
  }, [tunable]);

  // A live channel's window (#2088 decision 2): ←/→ and the chrome's ⟲/⟳ move 10 s through the
  // fifteen minutes the stream holds, Start over and the playbar's past name an instant, and L goes
  // live. Forward from live has nowhere to go. All of it goes through the caller's live transport,
  // which owns the element; without one (or without its `seek`) they do nothing.
  const seekLiveTo = (wallClockMs: number) => {
    if (liveTransport?.seek) void liveTransport.seek(wallClockMs);
  };
  const seekLiveBy = (deltaMs: number) => {
    if (!liveTransport || (deltaMs > 0 && liveTransport.state.mode === "live")) return;
    seekLiveTo(liveTransport.state.viewerTimeMs + deltaMs);
  };
  const goLive = () => {
    if (liveTransport) void liveTransport.goLive();
  };

  // Keyboard shortcuts — what native controls would have given free: Space/K toggle, M mutes, ←/→
  // seek (a live channel within its window), L goes live. Bound on the player's surface so they
  // cannot fire while it is off-screen, and skipped when focus is in a control that uses the same
  // keys (Space on a <button> must click). The channel keys are not among them: the document
  // listener above takes those, wherever focus is.
  const onKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    // Typing belongs to the field: a search box in the panel must be able to take "m" and "k".
    if (target.closest("input:not([type=range]), textarea, select")) return;
    const isSlider = target.getAttribute("role") === "slider";
    // The live chrome's controls are RN-web Pressables — `div[role=button]`, not <button> — and
    // they act on Space and Enter themselves, so the frame must not act on the key a second time.
    const isButton = Boolean(target.closest("button, [role=button]"));
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
        // The caller's own L comes first (Watch: the last channel, §9.3); Go live is the fallback.
        if (onShortcut && !e.metaKey && !e.ctrlKey && !e.altKey && onShortcut(e.key)) break;
        goLive();
        break;
      case "m":
        e.preventDefault();
        setMuted((m) => !m);
        break;
      case "ArrowUp":
      case "PageUp":
      case "ChannelUp":
      case "ArrowDown":
      case "PageDown":
      case "ChannelDown":
        // The document listener's (above); passing here keeps one press to one step, and keeps the
        // keys from reaching the caller's `onShortcut` below.
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
        style={{
          ...(fill ? { height: "100%" } : { aspectRatio: "16 / 9" }),
          backgroundColor: "black",
          outline: "none",
          overflow: "hidden",
          position: "relative",
          width: "100%",
        }}
        // Focusable by script only (not a Tab stop) so the mount claim above can land on the surface.
        tabIndex={-1}
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
          style={{
            cursor: controlsShown ? "pointer" : "none",
            display: "block",
            height: "100%",
            width: "100%",
          }}
          autoPlay={autoPlay}
          muted={muted}
          playsInline
          onClick={toggle}
          onPlaying={onPlaybackStart}
          // play/pause, elapsed, and metadata-load are owned by usePlaybackState.
          {...mediaHandlers}
        />

        {/* OVERLAY SLOT — a caller-rendered transient state (channel-watch's TunerLoader during
          warm-up). Sits directly over the video but UNDER the control-bar scrims below, so the
          controls stay operable. pointer-events-none so it never eats a click on the frame. */}
        {overlay && (
          <div style={{ inset: 0, pointerEvents: "none", position: "absolute", zIndex: 0 }}>{overlay}</div>
        )}

        {/* LIVE: the caller's chrome (§9), drawn from the player's own state. */}
        {live &&
          renderChrome?.({
            controlsShown,
            fullscreen,
            goLive,
            hide: hideControls,
            hold: holdControls.hold,
            muted,
            playing,
            seekBy: seekLiveBy,
            seekTo: seekLiveTo,
            setMuted,
            setVolume,
            toggle,
            toggleFullscreen,
            volume,
          })}

        {/* A CLIP: `leading` and the title over the top, the transport over the bottom, fading with
          the auto-hide. Dragging the level to zero mutes and off zero un-mutes, as every player does. */}
        {!live && (
          <ClipChrome
            density="pointer"
            durationMs={ready ? duration * 1000 : undefined}
            elapsedMs={current * 1000}
            fullscreen={fullscreen}
            leading={leading}
            muted={muted}
            onFullscreen={toggleFullscreen}
            onToggleMute={() => setMuted((m) => !m)}
            onTogglePlay={toggle}
            onVolume={(level) => {
              setVolume(level);
              setMuted(level === 0);
            }}
            playing={playing}
            pointer
            shown={controlsShown}
            title={title}
            volume={volume}
          />
        )}
      </div>
    </HoldControlsContext.Provider>
  );
};

export { VideoPlayer };
