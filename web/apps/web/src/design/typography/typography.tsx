import { typography } from "@loomarr/tokens";

// Typography — the type system documenting itself (frontend-design §2.2, §5.1a).
//
// Like the palette, every value is READ from the generated tokens. The scale is small and
// deliberate; a component reaching for a size that is not on it (`text-[10.5px]`) is a smell,
// and this page is where that becomes obvious rather than invisible.

// The rule that matters most, and the one most easily forgotten — it lives as a comment on the
// tokens today, which is a place nobody looks while writing a component.
const MONO_RULE = "If it came from a machine, it's mono.";

const SIZE_USE: Record<string, string> = {
  xs: "Captions, badge text, timestamps",
  sm: "Secondary copy, table cells",
  base: "Body — the default",
  md: "Emphasised body, card titles",
  lg: "Section headings",
  xl: "Page headings",
  "2xl": "Display — the wizard, empty states",
};

const Typography = () => (
  <div className="flex flex-col gap-8 p-6">
    <header className="flex flex-col gap-1">
      <h1 className="font-semibold text-xl">Typography</h1>
      <p className="max-w-2xl text-muted-foreground text-sm">
        Geist and Geist Mono, self-hosted (§2.2) — no webfont CDN, because the app must work on a LAN with no
        internet. Sizes come from the generated tokens; the samples below render at their real values.
      </p>
    </header>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">The two families</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2 rounded-md border border-border p-4">
          <div className="font-mono text-[11px] text-static-400 uppercase tracking-wide">Sans</div>
          <div className="text-lg" style={{ fontFamily: typography.fonts.sans }}>
            Saturday Morning Cartoons
          </div>
          <p className="text-muted-foreground text-sm">
            Everything a person wrote or would say aloud: titles, descriptions, prose, buttons.
          </p>
        </div>
        <div className="flex flex-col gap-2 rounded-md border border-border p-4">
          <div className="font-mono text-[11px] text-static-400 uppercase tracking-wide">Mono</div>
          <div className="font-mono text-lg" style={{ fontFamily: typography.fonts.mono }}>
            CH 42 · 21:30–22:00
          </div>
          <p className="text-muted-foreground text-sm">
            <strong className="text-foreground">{MONO_RULE}</strong> Channel numbers, clock times, durations,
            ids, state badges — anything the system emitted rather than authored.
          </p>
        </div>
      </div>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Scale</h2>
      <div className="flex flex-col divide-y divide-border">
        {Object.entries(typography.size).map(([name, px]) => (
          <div key={name} className="flex items-baseline gap-4 py-3">
            <span className="w-12 shrink-0 font-mono text-[11px] text-static-400">{name}</span>
            <span className="w-12 shrink-0 font-mono text-[11px] text-static-400 tabular-nums">
              {`${px}px`}
            </span>
            <span style={{ fontSize: px }} className="min-w-0 flex-1 truncate">
              Channels play what's ready
            </span>
            <span className="hidden shrink-0 text-muted-foreground text-xs sm:block">{SIZE_USE[name]}</span>
          </div>
        ))}
      </div>
      <p className="max-w-2xl text-muted-foreground text-sm">
        Seven sizes, and nothing between them. A component reaching for an off-scale value is usually solving
        a spacing problem with type — the scale is short on purpose so that shows up as a decision rather than
        a drift.
      </p>
    </section>

    <section className="flex flex-col gap-3">
      <h2 className="font-semibold text-base">Leading</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        {Object.entries(typography.leading).map(([name, value]) => (
          <div key={name} className="flex flex-col gap-2 rounded-md border border-border p-4">
            <div className="font-mono text-[11px] text-static-400 uppercase tracking-wide">
              {`${name} · ${value}`}
            </div>
            <p style={{ lineHeight: value }} className="text-sm">
              A channel is a wall clock, not a playlist that starts when someone watches. Tune in forty
              minutes into an hour-long film and you land forty minutes in.
            </p>
          </div>
        ))}
      </div>
    </section>
  </div>
);

export { Typography };
