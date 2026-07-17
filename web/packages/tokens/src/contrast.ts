// WCAG contrast math over the Test Card palette. The token generator runs this
// so a palette or tint-alpha edit that breaks an on-tint pairing FAILS the build
// (frontend-design §2.1 "the badge/tint rule, learned the hard way, twice";
// §5.3 "contrast enforced twice — here at build, and by axe on the rendered UI").

export type RGB = { r: number; g: number; b: number };

export function hexToRgb(hex: string): RGB {
  const h = hex.replace("#", "");
  const n = parseInt(
    h.length === 3
      ? h
          .split("")
          .map((c) => c + c)
          .join("")
      : h,
    16,
  );
  return { r: (n >> 16) & 0xff, g: (n >> 8) & 0xff, b: n & 0xff };
}

export function rgbToHex({ r, g, b }: RGB): string {
  const to = (v: number) =>
    Math.max(0, Math.min(255, Math.round(v)))
      .toString(16)
      .padStart(2, "0");
  return `#${to(r)}${to(g)}${to(b)}`.toUpperCase();
}

// WCAG relative luminance (sRGB → linearized).
function luminance({ r, g, b }: RGB): number {
  const lin = (c: number) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

// WCAG contrast ratio between two opaque colors (1..21).
export function contrast(fg: string, bg: string): number {
  const l1 = luminance(hexToRgb(fg));
  const l2 = luminance(hexToRgb(bg));
  const [hi, lo] = l1 >= l2 ? [l1, l2] : [l2, l1];
  return (hi + 0.05) / (lo + 0.05);
}

// Composite an accent tint — color-mix(in srgb, accent N%, transparent) — over an
// opaque surface, yielding the actual pixel color the eye (and axe) sees. This is
// the background badge text must pass 4.5:1 against.
export function compositeTint(accentHex: string, alphaPct: number, surfaceHex: string): string {
  const a = alphaPct / 100;
  const fg = hexToRgb(accentHex);
  const bg = hexToRgb(surfaceHex);
  return rgbToHex({
    r: a * fg.r + (1 - a) * bg.r,
    g: a * fg.g + (1 - a) * bg.g,
    b: a * fg.b + (1 - a) * bg.b,
  });
}

export const AA_SMALL = 4.5; // WCAG AA for small text (11px badges are small text)
