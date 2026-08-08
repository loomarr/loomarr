import * as SliderPrimitive from "@radix-ui/react-slider";
import { Pause, Play, Volume2, VolumeX } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib";
import type { VideoPlayerProps } from "./video-player.type";

// VideoPlayer — the app's own video surface, with Loomarr's controls rather than the browser's
// (V39, frontend-design §3 Layer 1).
//
// ⚠ **Custom controls rather than `<video controls>` — a maintainer decision (2026-08-03) that
// buys a real cost.** Native controls are keyboard-correct, screen-reader-correct and free;
// hand-built ones make every one of those this file's job. What they buy is a player that reads
// as part of the app, which matters wherever watching something is part of a decision rather than
// the point of the page. The consequences are handled explicitly below — keyboard shortcuts, an
// ARIA slider with real semantics, a focus route to every control — and are why this file is not
// short.
//
// ⚠ **Deliberately knows NOTHING about clips.** It takes a `src` and a `title`, both plain
// strings. The filler catalog wraps it in a dialog and hands it `clipMediaURL(clip.hash)`; the
// channel-watch surface the mock sketches plays a live stream through the same component. Putting
// `ClipDTO` in here would make "core primitive" a lie the first time something that is not a clip
// needed a player.

// The scrub step for arrow keys, in seconds. Five is the convention every video player uses and a
// useful fraction of a 30-second advert.
const SCRUB_STEP = 5;

// mmss renders a media time. Local rather than `formatClipDuration` from @loomarr/core: that one
// is about CLIPS, and this primitive is the one thing here that must not know what a clip is.
const mmss = (seconds: number): string => {
  const total = Math.max(0, Math.floor(seconds || 0));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

const VideoPlayer = ({ src, title, autoPlay, leading, live, attach, className }: VideoPlayerProps) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  const [current, setCurrent] = useState(0);
  // ⚠ Duration comes from the ELEMENT once loaded, never from a caller-supplied length. A caller
  // that has one (a clip's probed `durationMs`) can still disagree with the file — a
  // variable-frame-rate capture, or a file replaced on disk since the last scan. Scrubbing
  // against a length the element does not agree with puts the handle in the wrong place and makes
  // the end unreachable.
  const [duration, setDuration] = useState(0);
  // Distinguishes "still loading" from "0 seconds in": both are `current === 0`, and only one of
  // them should render as a dashed placeholder.
  const [ready, setReady] = useState(false);
  // Controls OVERLAY the video (Emby/Jellyfin-style) and auto-hide during playback. `controlsShown`
  // is the visibility; it is forced true while paused (a paused player always shows its controls,
  // like every media player) and hides a few seconds after the last interaction while playing.
  const [controlsShown, setControlsShown] = useState(true);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  // ⚠ Reset when the SOURCE changes. A player reused for a second video (a dialog reopened on a
  // different clip) would otherwise show the previous one's elapsed time and playing state until
  // the new metadata arrived.
  useEffect(() => {
    setPlaying(false);
    setCurrent(0);
    setDuration(0);
    setReady(false);
  }, []);

  // Custom source binding (hls.js, etc.). When `attach` is given, the primitive hands over the
  // element and runs the returned cleanup on unmount — so the element's lifecycle stays here while
  // WHAT plays through it is the caller's concern. Re-runs if `attach` identity changes.
  useEffect(() => {
    if (!attach) return;
    const el = videoRef.current;
    if (!el) return;
    return attach(el);
  }, [attach]);

  // revealControls shows the overlay and (re)arms the auto-hide. Called on mousemove/tap/focus over
  // the frame. While paused, controls stay up — matching every media player — so the hide timer is
  // only armed when playing.
  const revealControls = useCallback(() => {
    setControlsShown(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    if (playing) {
      hideTimer.current = setTimeout(() => setControlsShown(false), 2800);
    }
  }, [playing]);

  // A paused player always shows its controls; a playing one starts the hide countdown. Keeps the
  // overlay in sync when playback state flips without a pointer event (autoplay, the video ending).
  useEffect(() => {
    if (!playing) {
      setControlsShown(true);
      if (hideTimer.current) clearTimeout(hideTimer.current);
      return;
    }
    revealControls();
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current);
    };
  }, [playing, revealControls]);

  const seekTo = useCallback((seconds: number) => {
    const el = videoRef.current;
    if (!el) return;
    // Clamped against the ELEMENT's duration: seeking past the end throws in some browsers and
    // silently does nothing in others, and `NaN` (before metadata) would poison the expression
    // and freeze the handle.
    const max = Number.isFinite(el.duration) ? el.duration : 0;
    el.currentTime = Math.min(Math.max(0, seconds), max);
  }, []);

  const toggle = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    if (el.paused) {
      // ⚠ The promise is caught, not ignored. Autoplay policies reject `play()` when the gesture
      // heuristic is not satisfied, and an uncaught rejection is an unhandled promise error in
      // the console on a perfectly ordinary interaction.
      void el.play().catch(() => setPlaying(false));
    } else {
      el.pause();
    }
  }, []);

  // ⚠ **Keyboard shortcuts are not a nicety — they are what native controls would have given for
  // free.** Space/K toggle, arrows scrub, M mutes: the set every player has, so someone who has
  // used one already knows this one.
  //
  // Bound on the player's own surface rather than on `window`, so they cannot fire while it is
  // off-screen, and skipped when focus is in a control that uses the same keys itself (the slider
  // handles its own arrows; Space on a <button> must click it).
  const onKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    const isSlider = target.getAttribute("role") === "slider";
    const isButton = target.tagName === "BUTTON";

    switch (e.key) {
      case " ":
      case "k":
        if (isButton) return;
        e.preventDefault();
        toggle();
        break;
      case "ArrowLeft":
        // Live has nothing to seek to, so the scrub shortcuts are inert (the scrubber itself is
        // replaced by a LIVE indicator below).
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
      default:
        break;
    }
  };

  // ⚠ **The scrubber's keyboard contract, click-to-seek and drag are RADIX's, not ours** (§14,
  // V39). This file previously hand-rolled `role="slider"` with its own arrow/Home/End handling —
  // semantics that rot silently, because nothing fails when they drift, and a screen reader then
  // announces an affordance that does not work. Deleting ~40 lines of that in favour of a
  // primitive the app already uses for Select/Dialog/Tooltip is strictly less to get wrong.

  // ⚠ The keyboard handler below sits on a plain <div> on purpose: it is a SHORTCUT layer over a
  // composite widget, not a control itself. Every action it offers also has a focusable button or
  // slider inside, so making this wrapper focusable would add a tab stop that does nothing when
  // activated.
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: shortcut layer, see above
    <div
      className={cn("group/player relative aspect-video w-full overflow-hidden bg-black", className)}
      onKeyDown={onKeyDown}
      onMouseMove={revealControls}
      onMouseLeave={() => playing && setControlsShown(false)}
      onFocus={revealControls}
    >
      <div className="relative size-full bg-black">
        {/* ⚠ No <track>: callers pass archive footage and live streams, neither of which has a
            caption track to point at. An empty one would assert captions exist. (No suppression
            needed — this config does not enable `useMediaCaption`.) */}
        <video
          ref={videoRef}
          // `attach` owns the source when present (hls.js binds via attachMedia, not a src
          // attribute); otherwise the plain URL drives a native <video>.
          src={attach ? undefined : src}
          // cursor-pointer because the frame itself toggles playback (onClick below) — the
          // convention every video player follows. The cursor hides with the controls during
          // playback, exactly as Emby/Jellyfin do, so a still frame is not cluttered by an arrow.
          className={cn("size-full cursor-pointer", !controlsShown && "cursor-none")}
          autoPlay={autoPlay}
          muted={muted}
          playsInline
          onClick={toggle}
          onPlay={() => setPlaying(true)}
          onPause={() => setPlaying(false)}
          onTimeUpdate={(e) => setCurrent(e.currentTarget.currentTime)}
          onLoadedMetadata={(e) => {
            // Guarded: a live stream reports Infinity, and a bar computed from that renders a
            // handle at NaN% — which CSS drops, leaving it stuck at 0.
            const d = e.currentTarget.duration;
            setDuration(Number.isFinite(d) ? d : 0);
            setReady(true);
          }}
        />

        {/* Title top-right, `leading` top-left, both on a scrim: they sit over arbitrary video,
            which has NO guaranteed contrast behind text. The same legibility rule the clip card's
            overlays follow. Fades WITH the controls (Emby/Jellyfin hide their top chrome too). */}
        {(title || leading) && (
          <div
            className={cn(
              "pointer-events-none absolute top-0 right-0 left-0 flex items-start justify-between gap-2 bg-linear-to-b from-black/70 to-transparent p-3 transition-opacity duration-200",
              controlsShown ? "opacity-100" : "opacity-0",
            )}
          >
            <div className="pointer-events-auto shrink-0">{leading}</div>
            {title && (
              <p className="pointer-events-auto min-w-0 truncate text-right font-medium text-sm text-static-100">
                {title}
              </p>
            )}
          </div>
        )}

        {/* The control bar — OVERLAID on the video (Emby/Jellyfin-style), on a bottom scrim,
            auto-hiding during playback. Amber on the app's mono type, which is why these are
            hand-built rather than the browser's. */}
        <div
          className={cn(
            "absolute right-0 bottom-0 left-0 flex items-center gap-3 bg-linear-to-t from-black/80 via-black/40 to-transparent px-4 pt-8 pb-3 transition-opacity duration-200",
            controlsShown ? "opacity-100" : "pointer-events-none opacity-0",
          )}
        >
          {/* ⚠ The app's `Button` primitive, NOT a hand-rolled <button>. Everything a control
            needs — the pointer cursor Tailwind v4's Preflight stopped providing, the focus ring,
            the disabled handling — lives there once. Three hand-styled controls in this file each
            re-deriving `cursor-pointer` was the smell that said so. */}
          <Button
            variant="ghost"
            size="icon"
            onClick={toggle}
            // ⚠ The name changes with the STATE. A button permanently called "Play" lies as soon as
            // the video is playing, and a screen-reader user has no other way to know.
            aria-label={playing ? "Pause" : "Play"}
            className="size-8 shrink-0 rounded-full text-signal hover:bg-transparent hover:text-signal-400"
          >
            {playing ? (
              <Pause className="fill-current" aria-hidden />
            ) : (
              <Play className="fill-current" aria-hidden />
            )}
          </Button>

          {live ? (
            // LIVE: no scrubber. A live channel plays what is on — there is nothing to seek to
            // (§9.1) — so the seek bar is replaced by a non-interactive LIVE indicator and a filled
            // bar, which reads as "you are at the live edge" without offering a seek that does
            // nothing.
            <div className="flex flex-1 items-center gap-3">
              <span className="inline-flex shrink-0 items-center gap-1.5 rounded bg-onair px-2 py-0.5 font-mono text-[10px] text-static-0 uppercase tracking-wide">
                <span className="size-1.5 rounded-full bg-static-0 motion-safe:animate-pulse" aria-hidden />
                Live
              </span>
              <div className="h-1.5 flex-1 rounded-full bg-static-700" aria-hidden>
                <div className="h-full rounded-full bg-onair" style={{ width: "100%" }} />
              </div>
            </div>
          ) : (
            <>
              {/* Radix Slider (§14). It owns the WAI-ARIA contract, arrow/Home/End stepping, pointer
                capture, drag and click-anywhere-to-seek — all of which this file used to hand-roll.

                ⚠ `aria-valuetext` is still ours, and is the one thing Radix cannot infer: without
                it a screen reader reads `aria-valuenow` as a bare number of seconds ("4"), where
                what a listener needs is "0:04 of 0:30".

                ⚠ `max` is floored to at least 1. Radix divides by the range to place the thumb, so
                a max of 0 — every render before metadata arrives — yields NaN and React drops the
                style entirely, leaving the thumb stuck at the left edge even once playback starts. */}
              <SliderPrimitive.Root
                value={[current]}
                max={Math.max(1, duration)}
                step={0.1}
                onValueChange={([v]) => seekTo(v ?? 0)}
                className="group relative flex h-4 flex-1 cursor-pointer touch-none select-none items-center"
              >
                <SliderPrimitive.Track className="relative h-1.5 w-full grow rounded-full bg-static-700">
                  <SliderPrimitive.Range className="absolute h-full rounded-full bg-signal" />
                </SliderPrimitive.Track>
                <SliderPrimitive.Thumb
                  aria-label="Seek"
                  aria-valuetext={`${mmss(current)} of ${mmss(duration)}`}
                  className="block size-3 rounded-full bg-signal opacity-0 transition-opacity focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-signal focus-visible:ring-offset-2 focus-visible:ring-offset-card group-hover:opacity-100"
                />
              </SliderPrimitive.Root>

              {/* tabular-nums so the row does not jitter as the digits change — it sits beside a
                slider whose handle is already moving. */}
              <span className="shrink-0 font-mono text-static-300 text-xs tabular-nums">
                {ready ? `${mmss(current)} / ${mmss(duration)}` : "–:–– / –:––"}
              </span>
            </>
          )}

          <Button
            variant="ghost"
            size="icon"
            onClick={() => setMuted((m) => !m)}
            aria-label={muted ? "Unmute" : "Mute"}
            className="size-8 shrink-0 rounded-full text-static-300 hover:bg-transparent hover:text-static-100"
          >
            {muted ? <VolumeX aria-hidden /> : <Volume2 aria-hidden />}
          </Button>
        </div>
      </div>
    </div>
  );
};

export { VideoPlayer };
