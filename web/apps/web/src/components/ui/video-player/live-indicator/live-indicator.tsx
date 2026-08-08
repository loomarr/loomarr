// LiveIndicator — the red "● LIVE" badge (§9.1 Watch). A pulsing dot + the word, in the player's
// top-left (the mock). Marks a live channel stream as opposed to a seekable clip. `motion-safe`
// gates the pulse so a reduced-motion viewer gets a steady dot.
//
// A named component, not inline JSX — the player's top bar composes it (its own folder/story/test).
const LiveIndicator = () => (
  <span className="inline-flex shrink-0 items-center gap-1.5 rounded bg-onair px-2 py-0.5 font-mono text-[10px] text-static-0 uppercase tracking-wide">
    <span className="size-1.5 rounded-full bg-static-0 motion-safe:animate-pulse" aria-hidden />
    Live
  </span>
);

export { LiveIndicator };
