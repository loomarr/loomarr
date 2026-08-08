import type { GuideAiring } from "@loomarr/api";
import { useId, useRef, useState } from "react";
import { cn } from "@/lib";
import type { TimelineScrubberProps } from "./timeline-scrubber.type";

// TimelineScrubber — the live player's scrubber, a MINI GUIDE (§9.1 Watch, V47).
//
// A live channel cannot seek — it plays what is on — so a normal video scrubber would offer a drag
// that does nothing. Instead this shows WHERE YOU ARE in the schedule: the current programme with a
// live playhead, the next few, and the commercial breaks between them as their own segments.
// Hovering a point INSPECTS the timeline (a preview card: the episode, its window, its still image)
// rather than seeking. It spans the FULL WIDTH of the player's control bar, in its own row above the
// buttons (the mock's layout).
//
// It renders `GuideAiring[]` (each block a programme with episode name/series/season + a TMDB
// `thumbUrl`, or a `filler`/`flex` break). The breaks are REAL blocks in that data — no
// gap-synthesising — so the strip is a faithful slice of the guide. It is a pure renderer; the
// caller (channel-watch) fetches the airings.

// isBreak reports whether a block is a commercial break / dead-time rather than a programme. Filler
// and flex both read as "not a show" to a viewer; only a real programme gets a title + image.
const isBreak = (a: GuideAiring): boolean => a.kind === "filler" || a.kind === "flex";

// showName is the series (for an episode) or the movie title — the top line of the hover card.
const showName = (a: GuideAiring): string => {
  if (isBreak(a)) return "Commercial break";
  return a.series || a.title || "Programme";
};

// episodeName is the episode's own title, shown UNDER the series for a series block; "" otherwise
// (a movie's name is already the show line, a break has none).
const episodeName = (a: GuideAiring): string => (a.series && a.series !== a.title ? a.title : "");

// episodeTag renders "S2 E4" — a clean season/episode marker (no interpunct clutter). "" when the
// block is not a numbered series episode.
const episodeTag = (a: GuideAiring): string => (a.season && a.episode ? `S${a.season} E${a.episode}` : "");

// clockTime renders an epoch-ms as a short wall-clock (8:21 AM), the guide's own format.
const clockTime = (ms: number): string =>
  new Date(ms).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });

// durationLabel renders a span as "1h 30m" / "22m" — the guide's compact duration.
const durationLabel = (ms: number): string => {
  const mins = Math.round(ms / 60_000);
  if (mins < 60) return `${mins}m`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
};

const TimelineScrubber = ({ airings, nowMs, className }: TimelineScrubberProps) => {
  const now = nowMs ?? Date.now();
  const trackRef = useRef<HTMLDivElement>(null);
  // The inspected block, the pointer x (px, within the track) and the track's width — the last two
  // let the hover card CLAMP itself inside the strip instead of overflowing at the edges. Null = not
  // hovering.
  const [hover, setHover] = useState<{ block: GuideAiring; pointerX: number; trackW: number } | null>(null);
  const descId = useId();

  if (airings.length === 0) return null;

  // The strip spans from the first block's start to the last's stop — "now + next few". Each block's
  // width is its share of that total, so a 90-min show is wide and a 2-min break thin, exactly as a
  // proportional guide row reads.
  const spanStart = airings[0]?.startMs ?? now;
  const spanEnd = airings[airings.length - 1]?.stopMs ?? now;
  const spanTotal = Math.max(1, spanEnd - spanStart);
  const playheadFrac = Math.min(1, Math.max(0, (now - spanStart) / spanTotal));

  // onMove maps the pointer x to a fraction of the strip, then to the block under it — the INSPECT
  // interaction (no seek). Reads clientX against the track's own rect so it is correct regardless of
  // page scroll or the player's position, and records the pixel x + width for edge-clamping the card.
  const onMove = (e: React.PointerEvent) => {
    const rect = trackRef.current?.getBoundingClientRect();
    if (!rect || rect.width === 0) return;
    const pointerX = Math.min(rect.width, Math.max(0, e.clientX - rect.left));
    const frac = pointerX / rect.width;
    const atMs = spanStart + frac * spanTotal;
    const block = airings.find((a) => atMs >= a.startMs && atMs < a.stopMs) ?? airings[airings.length - 1];
    if (block) setHover({ block, pointerX, trackW: rect.width });
  };

  // CARD_WIDTH matches the card's `w-64` (256px). The card is centered on the pointer but its left is
  // clamped so neither edge crosses the track — near the ends it "sticks" to the edge, staying fully
  // visible (the arrow-less card follows the pointer through the middle, then pins at the extremes).
  const CARD_WIDTH = 256;
  const cardLeft = hover
    ? Math.min(Math.max(hover.pointerX, CARD_WIDTH / 2), hover.trackW - CARD_WIDTH / 2)
    : 0;

  return (
    <div className={cn("relative w-full", className)}>
      {/* The hover-preview card — the episode at the inspected point, with its TMDB still, positioned
          over the pointer. Not interactive (pointer-events-none) so it never eats the move events. */}
      {hover && (
        <div
          role="tooltip"
          id={descId}
          className="pointer-events-none absolute bottom-full z-20 mb-2 w-64 -translate-x-1/2 overflow-hidden rounded-lg border border-border bg-popover shadow-2xl"
          // Clamped pixel left (not a % of the pointer) so the card never overflows the strip's edges.
          style={{ left: `${cardLeft}px` }}
        >
          {!isBreak(hover.block) && hover.block.thumbUrl && (
            // The episode still is the hero — a 16:9 image the card is built around. A subtle bottom
            // gradient lets the S/E badge sit over it. object-cover fills the width without letterbox.
            <div className="relative aspect-video w-full overflow-hidden bg-static-800">
              <img src={hover.block.thumbUrl} alt="" className="size-full object-cover" />
              {episodeTag(hover.block) && (
                <span className="absolute right-2 bottom-2 rounded bg-black/70 px-1.5 py-0.5 font-mono text-[10px] text-static-0 backdrop-blur-sm">
                  {episodeTag(hover.block)}
                </span>
              )}
            </div>
          )}
          <div className="flex flex-col gap-1 px-3 py-2.5">
            {/* Series/movie name (top). When there is NO thumbnail, the S/E badge rides here instead
                of on the image, so it is never lost. */}
            <div className="flex items-start justify-between gap-2">
              <p className="font-semibold text-popover-foreground text-sm leading-snug">
                {showName(hover.block)}
              </p>
              {!hover.block.thumbUrl && episodeTag(hover.block) && (
                <span className="shrink-0 rounded bg-static-800 px-1.5 py-0.5 font-mono text-2xs text-muted-foreground">
                  {episodeTag(hover.block)}
                </span>
              )}
            </div>
            {/* Episode title — wraps to two lines rather than truncating, so a long name is readable. */}
            {episodeName(hover.block) && (
              <p className="line-clamp-2 text-popover-foreground/90 text-xs leading-snug">
                {episodeName(hover.block)}
              </p>
            )}
            <p className="mt-0.5 font-mono text-2xs text-muted-foreground">
              {clockTime(hover.block.startMs)}–{clockTime(hover.block.stopMs)} ·{" "}
              {durationLabel(hover.block.stopMs - hover.block.startMs)}
            </p>
          </div>
        </div>
      )}

      {/* The strip: proportional blocks, a live playhead, inspect-on-hover. */}
      <div
        ref={trackRef}
        className="group relative flex h-2 w-full cursor-default overflow-hidden rounded-full bg-static-800"
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
        aria-describedby={hover ? descId : undefined}
      >
        {airings.map((a) => {
          const widthPct = ((a.stopMs - a.startMs) / spanTotal) * 100;
          const current = now >= a.startMs && now < a.stopMs;
          return (
            <div
              // startMs+kind is a stable identity: airtimes are unique per block and never collide
              // within one strip, so no array index is needed.
              key={`${a.startMs}-${a.kind}`}
              className={cn(
                "h-full border-static-900 border-r transition-opacity last:border-r-0",
                isBreak(a) ? "bg-static-700" : current ? "bg-signal" : "bg-static-500 group-hover:opacity-90",
              )}
              style={{ width: `${widthPct}%` }}
            />
          );
        })}
        {/* The live playhead — a white tick at "now", the one point that is real rather than inspected. */}
        <div
          className="pointer-events-none absolute -top-0.5 -bottom-0.5 w-0.5 rounded-full bg-static-0"
          style={{ left: `${(playheadFrac * 100).toFixed(3)}%` }}
          aria-hidden
        />
      </div>
    </div>
  );
};

export { TimelineScrubber };
