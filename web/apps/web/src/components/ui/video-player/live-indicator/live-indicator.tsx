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
// A named component, not inline JSX — the player's top bar composes it (its own folder/story/test).
const LiveIndicator = () => (
  <span className="inline-flex shrink-0 items-center gap-1.5 rounded bg-onair-tint-15 px-2 py-0.5 font-mono text-[10px] text-onair-300 uppercase tracking-wide">
    <span className="size-1.5 rounded-full bg-onair-300 motion-safe:animate-pulse" aria-hidden />
    Live
  </span>
);

export { LiveIndicator };
