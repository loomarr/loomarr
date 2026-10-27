// LiveIndicator — the compact live-edge control in the playback bar (§9.1 Watch). At the edge it
// is a quiet "● Live" status. Behind the edge it becomes one cohesive capsule: delayed context on
// the left and the recovery action on the right. Keeping those together makes the action's result
// obvious and gives keyboard/remote users one predictable place to find transport state.
//
// ⚠ **Contrast delta from the mock (frontend-design §7).** The mock draws this as white on a solid
// `onair` fill (#fff on #E5484D at the mock's 10px) — 3.91:1, below WCAG-AA's 4.5:1 for small text, which the
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
      <span className="border-onair-300/25 bg-onair-tint-15 text-2xs text-onair-300 inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border px-2.5 font-mono tracking-[0.12em] uppercase shadow-sm backdrop-blur-sm">
        <span className="bg-onair-300 size-1.5 rounded-full motion-safe:animate-pulse" aria-hidden />
        Live
      </span>
    );
  }

  return (
    <span className="border-static-500/50 bg-static-900/80 inline-flex h-7 shrink-0 overflow-hidden rounded-full border shadow-sm backdrop-blur-sm">
      <span className="text-2xs text-static-100 inline-flex items-center gap-1.5 px-2.5 font-mono tracking-[0.08em] uppercase tabular-nums">
        {state.mode === "paused" && <span className="text-static-50 font-semibold">Paused</span>}
        {state.mode === "paused" && <span className="text-static-500">·</span>}
        <span>{lagLabel(state.lagSeconds)} behind</span>
      </span>
      <button
        type="button"
        onClick={onGoLive}
        className="border-static-500/50 bg-onair-tint-15 text-2xs text-onair-300 hover:bg-onair-tint-25 focus-visible:bg-onair-tint-25 focus-visible:ring-onair-300 inline-flex cursor-pointer items-center gap-1.5 border-l px-2.5 font-semibold tracking-[0.08em] uppercase transition-colors outline-none focus-visible:ring-2 focus-visible:ring-inset"
      >
        <span className="bg-onair-300 size-1.5 rounded-full" aria-hidden />
        Go live
      </button>
    </span>
  );
};

export type { LiveIndicatorProps };
export { LiveIndicator, lagLabel };
