import { motion, radius, rowHeight, space } from "@loomarr/tokens";

// Spacing, radius and motion — the geometry half of the system (§2.3, §2.4, §5.1a).
//
// Read from the generated tokens like every other Design page. Space is a 4px grid: the values
// are not arbitrary, they are multiples, and seeing them stacked is what makes an off-grid
// `gap-[7px]` obviously wrong rather than merely unusual.

const Spacing = () => (
  <div className="flex flex-col gap-8 p-6">
    <header className="flex flex-col gap-1">
      <h1 className="font-semibold text-xl">Spacing, radius & motion</h1>
      <p className="max-w-2xl text-muted-foreground text-sm">
        A 4px grid. Every step is a multiple of four, which is what makes the interface feel aligned without
        anyone measuring — and what makes an off-grid value stand out.
      </p>
    </header>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Space</h2>
      <div className="flex flex-col gap-2">
        {Object.entries(space).map(([step, px]) => (
          <div key={step} className="flex items-center gap-4">
            <span className="w-8 shrink-0 font-mono text-2xs text-static-400">{step}</span>
            <span className="w-12 shrink-0 font-mono text-2xs text-static-400 tabular-nums">{`${px}px`}</span>
            <div className="h-4 rounded-sm bg-signal-tint-30" style={{ width: px }} />
          </div>
        ))}
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Radius</h2>
      <div className="flex flex-wrap gap-4">
        {Object.entries(radius).map(([name, px]) => (
          <div key={name} className="flex flex-col items-center gap-2">
            <div className="size-16 border border-border bg-static-800" style={{ borderRadius: px }} />
            <span className="font-mono text-2xs text-static-400">{`${name} · ${px}px`}</span>
          </div>
        ))}
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Density</h2>
      <div className="flex flex-col gap-2">
        <div
          className="flex items-center rounded-md border border-border px-3"
          style={{ height: rowHeight.compact }}
        >
          <span className="text-sm">A table row at compact density</span>
          <span className="ml-auto font-mono text-2xs text-static-400">{`${rowHeight.compact}px`}</span>
        </div>
        <p className="max-w-2xl text-muted-foreground text-sm">
          Tables use the compact row height. A household's library runs to thousands of titles, so rows are
          sized to fit a screenful rather than to feel spacious.
        </p>
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Motion</h2>
      <div className="flex flex-col gap-2">
        {Object.entries(motion.duration).map(([name, value]) => (
          <div key={name} className="flex items-center gap-4">
            <span className="w-14 shrink-0 font-mono text-2xs text-static-400">{name}</span>
            <span className="w-16 shrink-0 font-mono text-2xs text-static-400">{value}</span>
            <span className="text-muted-foreground text-sm">
              {name === "fast" ? "Hover, focus, small state flips" : "Panels, disclosure, page transitions"}
            </span>
          </div>
        ))}
        <div className="flex items-center gap-4">
          <span className="w-14 shrink-0 font-mono text-2xs text-static-400">ease</span>
          <span className="shrink-0 font-mono text-2xs text-static-400">{motion.ease}</span>
        </div>
        <p className="max-w-2xl text-muted-foreground text-sm">
          One ease-out curve everywhere. Motion is short and decelerating — it should confirm that something
          happened, never make anyone wait for it. Under{" "}
          <code className="font-mono text-xs">prefers-reduced-motion</code> these collapse to near zero
          (§2.4), which the visual suite relies on to snapshot deterministically.
        </p>
      </div>
    </section>
  </div>
);

export { Spacing };
