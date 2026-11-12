import { spaceSteps } from "@loomarr/design-system";
import { webMotion, webRadius, webRowHeight, webSpace } from "@loomarr/design-system/web-theme";

// Spacing, radius and motion — the geometry half of the system (§2.3, §2.4, §5.1a).
//
// Read from the generated tokens like every other Design page. Space is a 4px grid: the values
// are not arbitrary, they are multiples, and seeing them stacked is what makes an off-grid
// `gap-[7px]` obviously wrong rather than merely unusual. The half steps the app already uses sit
// in the same list at their own size, and the copy names them as the recorded exceptions they are.

const steps = Object.entries(spaceSteps).sort(([, a], [, b]) => a - b);
const halfSteps = Object.keys(spaceSteps).filter((step) => !(step in webSpace));

const Spacing = () => (
  <div className="flex flex-col gap-8 p-6">
    <header className="flex flex-col gap-1">
      <h1 className="text-xl font-semibold">Spacing, radius & motion</h1>
      <p className="text-muted-foreground max-w-2xl text-sm">
        A 4px grid. Every step is a multiple of four, which is what makes the interface feel aligned without
        anyone measuring — and what makes an off-grid value stand out. The half steps ({halfSteps.join(", ")})
        are the exception: tight spacing inside a control or a chip, kept at its own size so a layout that
        uses one does not move when it adopts the scale.
      </p>
    </header>

    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Space</h2>
      <div className="flex flex-col gap-2">
        {steps.map(([step, px]) => (
          <div key={step} className="flex items-center gap-4">
            <span className="text-2xs text-static-400 w-8 shrink-0 font-mono">{step}</span>
            <span className="text-2xs text-static-400 w-12 shrink-0 font-mono tabular-nums">{`${px}px`}</span>
            <div className="bg-signal-tint-30 h-4 rounded-sm" style={{ width: px }} />
          </div>
        ))}
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Radius</h2>
      <div className="flex flex-wrap gap-4">
        {Object.entries(webRadius).map(([name, px]) => (
          <div key={name} className="flex flex-col items-center gap-2">
            <div className="border-border bg-static-800 size-16 border" style={{ borderRadius: px }} />
            <span className="text-2xs text-static-400 font-mono">{`${name} · ${px}px`}</span>
          </div>
        ))}
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Density</h2>
      <div className="flex flex-col gap-2">
        <div
          className="border-border flex items-center rounded-md border px-3"
          style={{ height: webRowHeight.compact }}
        >
          <span className="text-sm">A table row at compact density</span>
          <span className="text-2xs text-static-400 ml-auto font-mono">{`${webRowHeight.compact}px`}</span>
        </div>
        <p className="text-muted-foreground max-w-2xl text-sm">
          Tables use the compact row height. A household's library runs to thousands of titles, so rows are
          sized to fit a screenful rather than to feel spacious.
        </p>
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold">Motion</h2>
      <div className="flex flex-col gap-2">
        {Object.entries(webMotion.duration).map(([name, value]) => (
          <div key={name} className="flex items-center gap-4">
            <span className="text-2xs text-static-400 w-14 shrink-0 font-mono">{name}</span>
            <span className="text-2xs text-static-400 w-16 shrink-0 font-mono">{value}</span>
            <span className="text-muted-foreground text-sm">
              {name === "fast" ? "Hover, focus, small state flips" : "Panels, disclosure, page transitions"}
            </span>
          </div>
        ))}
        <div className="flex items-center gap-4">
          <span className="text-2xs text-static-400 w-14 shrink-0 font-mono">ease</span>
          <span className="text-2xs text-static-400 shrink-0 font-mono">{webMotion.ease}</span>
        </div>
        <p className="text-muted-foreground max-w-2xl text-sm">
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
