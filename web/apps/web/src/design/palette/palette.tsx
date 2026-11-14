import { Card, Grid, Stack, Text } from "@loomarr/design-system";
import {
  AA_SMALL,
  accents,
  compositeTint,
  contrast,
  staticScale,
  tintSteps,
  tintSurface,
} from "@loomarr/design-system/web-theme";

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
  <Text fontVariant={["tabular-nums"]} textRole="mono" tone={value >= AA_SMALL ? "success" : "danger"}>
    {value >= AA_SMALL ? `${value.toFixed(2)} AA` : `${value.toFixed(2)} FAIL`}
  </Text>
);

// `measure` is optional because not every swatch is a TEXT colour. Rendering "1.00 FAIL" for
// static-950 on static-950 would be measuring a surface against itself — a number with no
// meaning, and worse than no number, since it reads as a defect.
const Swatch = ({ name, hex, measure = true }: { name: string; hex: string; measure?: boolean }) => (
  <Stack align="center" direction="row" gap="control">
    <Card backgroundColor={hex} flexShrink={0} height={40} width={40} />
    <Stack flex={1} minWidth={0}>
      <Text numberOfLines={1} textRole="label">
        {name}
      </Text>
      <Text textRole="mono" tone="muted">
        {hex}
      </Text>
    </Stack>
    {measure && (
      <Stack align="end">
        <Text textRole="mono" tone="muted">
          on bg
        </Text>
        <Verdict value={contrast(hex, staticScale["static-950"])} />
      </Stack>
    )}
  </Stack>
);

// Which statics are SURFACES (measuring them as text is meaningless) versus text stops. The
// scale runs dark→light, and the dark end is what things sit ON.
const SURFACE_STATICS = new Set(["static-950", "static-900", "static-800", "static-700"]);

// The tint steps that carry TEXT. 8/12/15 are badge backgrounds and the badge rule governs
// them; 30/40 are fills only (§2.1) and nothing in the codebase puts accent text on them.
const isTextStep = (step: number) => step <= 15;

// The tint ramp for one accent, with the badge-text pairing measured on each step.
//
// This is the page's real payload: it renders the exact composite the contrast gate checks, so
// the -300 rule stops being a sentence someone has to have read and becomes a column you can
// see failing.
const TintRamp = ({ accent, hex, textHex }: { accent: string; hex: string; textHex: string }) => (
  <Stack gap="1">
    <Text textRole="hint" tone="muted">{`${accent} tints`}</Text>
    <Stack direction="row" gap="inline" wrap>
      {tintSteps.map((step) => {
        const composited = compositeTint(hex, step, tintSurface);
        const r = contrast(textHex, composited);
        return (
          // Labels and verdicts sit OUTSIDE the tint, on the page surface — muted grey on the
          // darker composites measures 2.98:1, i.e. this page breaking the rule it documents.
          //
          // TEXT SAMPLES ONLY ON THE TEXT STEPS. 8/12/15 are what badges use, so they show the
          // real pairing. 30/40 are FILL-ONLY in the codebase (a pod segment, a progress bar),
          // and rendering sample text on them would both misrepresent how they are used and
          // make this page fail the a11y gate for a violation it was only demonstrating. The
          // measured ratio is still printed for every step, so the scope of the rule stays
          // visible without being enacted.
          <Stack gap="1" key={step}>
            <Text textRole="mono" tone="muted">{`${step}%`}</Text>
            <Card
              alignItems="center"
              backgroundColor={composited}
              flexDirection="row"
              height={32}
              paddingHorizontal={8}
            >
              {isTextStep(step) ? (
                <Text style={{ color: textHex }} textRole="mono">
                  Badge
                </Text>
              ) : (
                <Text style={{ color: "transparent" }} textRole="mono">
                  fill
                </Text>
              )}
            </Card>
            <Verdict value={r} />
          </Stack>
        );
      })}
    </Stack>
  </Stack>
);

const Palette = () => {
  // Each accent already carries its validated on-tint text stop: `on` differs from `base`
  // exactly where the base fails on the composited 15% tint (onair, suggest → their -300
  // stops). Reading it rather than re-deriving it means this page shows the pairing the
  // generator actually validates, not a second opinion that could disagree with it.
  const accentRows = Object.entries(accents);

  return (
    <Stack gap="screen" padding="section">
      <Stack gap="1" maxWidth={672} render="header">
        <Text as="h1" textRole="title">
          Palette
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          The broadcast accents, drawn from the SMPTE bars. Every hex and every ratio on this page is read
          from the generated tokens and measured with the same function the CI contrast gate uses — so what
          you see here is what the build enforces.
        </Text>
      </Stack>

      <Stack gap="control" render="section">
        <Text as="h2" textRole="heading">
          Accents
        </Text>
        <Grid columns={{ base: 1, sm: 2 }} gap="3">
          {accentRows.map(([name, a]) => (
            <Swatch key={name} name={name} hex={a.base} />
          ))}
        </Grid>
      </Stack>

      <Stack gap="4" render="section">
        <Stack gap="1" maxWidth={672}>
          <Text as="h2" textRole="heading">
            Tints, and the badge rule
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            A tint is an alpha wash, not a fixed hex:{" "}
            <Text textRole="mono">color-mix(in srgb, accent N%, transparent)</Text> over the surface. 11px
            badge text is <em>small text</em> under WCAG, so the 4.5:1 bar applies. On the composited tints
            some accents fail at their base stop and use a <Text textRole="mono">-300</Text> stop instead —
            the swatches below show which, measured rather than asserted.
          </Text>
        </Stack>
        {accentRows.map(([name, a]) => (
          <TintRamp key={name} accent={name} hex={a.base} textHex={a.on} />
        ))}
        <Stack maxWidth={672}>
          <Text as="p" textRole="compact" tone="secondary">
            ⚠ Note where the ramps go red. The badge rule and its CI check are scoped to the{" "}
            <Text as="strong" strong textRole="compact" tone="primary">
              15% step
            </Text>{" "}
            — the one badges actually use — and accent text does not clear AA on the 30% and 40% steps. Those
            darker steps are for <em>fills</em> (a pod segment, a progress bar), never for accent text, and
            today nothing uses them that way. This page is where that stops being an unwritten assumption.
          </Text>
        </Stack>
      </Stack>

      <Stack gap="control" render="section">
        <Text as="h2" textRole="heading">
          Statics
        </Text>
        <Grid columns={{ base: 1, sm: 2 }} gap="3">
          {Object.entries(staticScale).map(([name, hex]) => (
            <Swatch key={name} name={name} hex={hex as string} measure={!SURFACE_STATICS.has(name)} />
          ))}
        </Grid>
        <Stack maxWidth={672}>
          <Text as="p" textRole="compact" tone="secondary">
            <Text textRole="mono">static-500</Text> sits at{" "}
            {ratio(staticScale["static-500"], staticScale["static-900"])}
            :1 on cards — below the AA bar — so it is restricted to disabled states and decorative glyphs. Any
            text carrying information uses <Text textRole="mono">static-400</Text> or better.
          </Text>
        </Stack>
      </Stack>
    </Stack>
  );
};

export { Palette };
