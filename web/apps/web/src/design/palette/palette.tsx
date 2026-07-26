import {
  AA_SMALL,
  accents,
  compositeTint,
  contrast,
  staticScale,
  tintSteps,
  tintSurface,
} from "@loomarr/tokens";
import { cn } from "@/lib";

// Palette — the design system documenting itself (frontend-design §2.1, §5.1a).
//
// EVERY VALUE ON THIS PAGE IS READ FROM THE GENERATED TOKENS AND MEASURED AT RENDER TIME.
// Nothing here is retyped from the design doc, and the ratios come from the SAME `contrast()`
// the CI gate uses — so this page cannot claim a pairing passes when the build says it fails.
// A hand-typed hex in a swatch would be exactly the drift §2.5 exists to prevent.

const ratio = (fg: string, bg: string) => contrast(fg, bg).toFixed(2);

// A pairing's verdict against WCAG AA for SMALL text. 11px badge copy is small text no matter
// how label-like it feels — the rule §2.1 calls "learned the hard way, twice".
const Verdict = ({ value }: { value: number }) => (
  <span
    className={cn("font-mono text-[10px] tabular-nums", value >= AA_SMALL ? "text-lock" : "text-onair-300")}
  >
    {value >= AA_SMALL ? `${value.toFixed(2)} AA` : `${value.toFixed(2)} FAIL`}
  </span>
);

// `measure` is optional because not every swatch is a TEXT colour. Rendering "1.00 FAIL" for
// static-950 on static-950 would be measuring a surface against itself — a number with no
// meaning, and worse than no number, since it reads as a defect.
const Swatch = ({ name, hex, measure = true }: { name: string; hex: string; measure?: boolean }) => (
  <div className="flex items-center gap-3">
    <div className="size-10 shrink-0 rounded-md border border-border" style={{ backgroundColor: hex }} />
    <div className="min-w-0">
      <div className="truncate font-medium text-sm">{name}</div>
      <div className="font-mono text-[11px] text-static-400 uppercase">{hex}</div>
    </div>
    {measure && (
      <div className="ml-auto text-right">
        <div className="font-mono text-[10px] text-static-400">on bg</div>
        <Verdict value={contrast(hex, staticScale["static-950"])} />
      </div>
    )}
  </div>
);

// Which statics are SURFACES (measuring them as text is meaningless) versus text stops. The
// scale runs dark→light, and the dark end is what things sit ON.
const SURFACE_STATICS = new Set(["static-950", "static-900", "static-800", "static-700"]);

// The tint ramp for one accent, with the badge-text pairing measured on each step.
//
// This is the page's real payload: it renders the exact composite the contrast gate checks, so
// the -300 rule stops being a sentence someone has to have read and becomes a column you can
// see failing.
const TintRamp = ({ accent, hex, textHex }: { accent: string; hex: string; textHex: string }) => (
  <div className="flex flex-col gap-1">
    <div className="font-mono text-[10px] text-static-400 uppercase tracking-wide">{`${accent} tints`}</div>
    <div className="flex flex-wrap gap-2">
      {tintSteps.map((step) => {
        const composited = compositeTint(hex, step, tintSurface);
        const r = contrast(textHex, composited);
        return (
          <div
            key={step}
            className="flex min-w-24 flex-col gap-1 rounded-md border border-border px-2 py-1.5"
            style={{ backgroundColor: composited }}
          >
            <span className="font-mono text-[10px] text-static-400">{`${step}%`}</span>
            <span className="font-mono text-[11px] uppercase" style={{ color: textHex }}>
              Badge
            </span>
            <Verdict value={r} />
          </div>
        );
      })}
    </div>
  </div>
);

const Palette = () => {
  // Each accent already carries its validated on-tint text stop: `on` differs from `base`
  // exactly where the base fails on the composited 15% tint (onair, suggest → their -300
  // stops). Reading it rather than re-deriving it means this page shows the pairing the
  // generator actually validates, not a second opinion that could disagree with it.
  const accentRows = Object.entries(accents);

  return (
    <div className="flex flex-col gap-8 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="font-semibold text-xl">Palette</h1>
        <p className="max-w-2xl text-muted-foreground text-sm">
          The broadcast accents, drawn from the SMPTE bars. Every hex and every ratio on this page is read
          from the generated tokens and measured with the same function the CI contrast gate uses — so what
          you see here is what the build enforces.
        </p>
      </header>

      <section className="flex flex-col gap-3">
        <h2 className="font-semibold text-base">Accents</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          {accentRows.map(([name, a]) => (
            <Swatch key={name} name={name} hex={a.base} />
          ))}
        </div>
      </section>

      <section className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <h2 className="font-semibold text-base">Tints, and the badge rule</h2>
          <p className="max-w-2xl text-muted-foreground text-sm">
            A tint is an alpha wash, not a fixed hex:{" "}
            <code className="font-mono text-xs">color-mix(in srgb, accent N%, transparent)</code> over the
            surface. 11px badge text is <em>small text</em> under WCAG, so the 4.5:1 bar applies. On the
            composited tints some accents fail at their base stop and use a{" "}
            <code className="font-mono text-xs">-300</code> stop instead — the swatches below show which,
            measured rather than asserted.
          </p>
        </div>
        {accentRows.map(([name, a]) => (
          <TintRamp key={name} accent={name} hex={a.base} textHex={a.on} />
        ))}
        <p className="max-w-2xl text-muted-foreground text-sm">
          ⚠ Note where the ramps go red. The badge rule and its CI check are scoped to the{" "}
          <strong className="text-foreground">15% step</strong> — the one badges actually use — and accent
          text does not clear AA on the 30% and 40% steps. Those darker steps are for <em>fills</em> (a pod
          segment, a progress bar), never for accent text, and today nothing uses them that way. This page is
          where that stops being an unwritten assumption.
        </p>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="font-semibold text-base">Statics</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          {Object.entries(staticScale).map(([name, hex]) => (
            <Swatch key={name} name={name} hex={hex as string} measure={!SURFACE_STATICS.has(name)} />
          ))}
        </div>
        <p className="max-w-2xl text-muted-foreground text-sm">
          <code className="font-mono text-xs">static-500</code> sits at{" "}
          {ratio(staticScale["static-500"], staticScale["static-900"])}
          :1 on cards — below the AA bar — so it is restricted to disabled states and decorative glyphs. Any
          text carrying information uses <code className="font-mono text-xs">static-400</code> or better.
        </p>
      </section>
    </div>
  );
};

export { Palette };
