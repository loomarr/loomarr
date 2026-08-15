// LiveIndicator — the "● LIVE" badge (§9.1 Watch). A pulsing dot + the word, in the player's
// top-left (the mock). Marks a live channel stream as opposed to a seekable clip. `motion-safe`
// gates the pulse so a reduced-motion viewer gets a steady dot.
//
// ⚠ **Contrast delta from the mock (frontend-design §7).** The mock draws this as white on a solid
// `onair` fill (#fff on #E5484D at 10px) — 3.91:1, below WCAG-AA's 4.5:1 for small text, which the
// axe gate blocks. So the shipped badge uses the design system's own AA-safe badge idiom instead
// (the `bg-onair-tint-15 text-onair-300` chip, as channel-card's error state does): a colored label
// on a translucent tint, contrast validated by the token generator. Same red read, legible.
//
import type { LivePlaybackState } from "../live-playback-transport.type";

interface LiveIndicatorProps {
  state: LivePlaybackState;
  onGoLive: () => void;
}

const lagLabel = (seconds: number): string => {
  const total = Math.max(0, Math.round(seconds));
  if (total < 60) return `${total}s`;
  return `${Math.floor(total / 60)}m ${total % 60}s`;
};

// A named component, not inline JSX — the player's top bar composes it (its own folder/story/test).
const LiveIndicator = ({ state, onGoLive }: LiveIndicatorProps) => {
  if (state.mode === "live") {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded bg-onair-tint-15 px-2 py-0.5 font-mono text-[10px] text-onair-300 uppercase tracking-wide">
        <span className="size-1.5 rounded-full bg-onair-300 motion-safe:animate-pulse" aria-hidden />
        Live
      </span>
    );
  }

  return (
    <span className="pointer-events-auto inline-flex shrink-0 items-center gap-2">
      <span className="rounded bg-static-800/90 px-2 py-0.5 font-mono text-[10px] text-static-100 uppercase tracking-wide">
        {state.mode === "paused" ? "Paused · " : ""}
        {lagLabel(state.lagSeconds)} behind
      </span>
      <button
        type="button"
        onClick={onGoLive}
        className="rounded bg-onair-tint-15 px-2 py-0.5 font-mono text-[10px] text-onair-300 uppercase tracking-wide outline-none hover:bg-onair-tint-25 focus-visible:ring-2 focus-visible:ring-onair-300"
      >
        Go live
      </button>
    </span>
  );
};

export type { LiveIndicatorProps };
export { LiveIndicator, lagLabel };
