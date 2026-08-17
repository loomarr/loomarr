// LiveIndicator — the compact live-edge control in the playback bar (§9.1 Watch). At the edge it
// is a quiet "● Live" status. Behind the edge it becomes one cohesive capsule: delayed context on
// the left and the recovery action on the right. Keeping those together makes the action's result
// obvious and gives keyboard/remote users one predictable place to find transport state.
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
      <span className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-onair-300/25 bg-onair-tint-15 px-2.5 font-mono text-[10px] text-onair-300 uppercase tracking-[0.12em] shadow-sm backdrop-blur-sm">
        <span className="size-1.5 rounded-full bg-onair-300 motion-safe:animate-pulse" aria-hidden />
        Live
      </span>
    );
  }

  return (
    <span className="inline-flex h-7 shrink-0 overflow-hidden rounded-full border border-static-500/50 bg-static-900/80 shadow-sm backdrop-blur-sm">
      <span className="inline-flex items-center gap-1.5 px-2.5 font-mono text-[10px] text-static-100 uppercase tabular-nums tracking-[0.08em]">
        {state.mode === "paused" && <span className="font-semibold text-static-50">Paused</span>}
        {state.mode === "paused" && <span className="text-static-500">·</span>}
        <span>{lagLabel(state.lagSeconds)} behind</span>
      </span>
      <button
        type="button"
        onClick={onGoLive}
        className="inline-flex cursor-pointer items-center gap-1.5 border-static-500/50 border-l bg-onair-tint-15 px-2.5 font-semibold text-[10px] text-onair-300 uppercase tracking-[0.08em] outline-none transition-colors hover:bg-onair-tint-25 focus-visible:bg-onair-tint-25 focus-visible:ring-2 focus-visible:ring-onair-300 focus-visible:ring-inset"
      >
        <span className="size-1.5 rounded-full bg-onair-300" aria-hidden />
        Go live
      </button>
    </span>
  );
};

export type { LiveIndicatorProps };
export { LiveIndicator, lagLabel };
