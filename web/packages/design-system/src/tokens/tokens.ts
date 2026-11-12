import nativeFonts from "../../native-fonts.json";
import brandContract from "./brand-contract.json";

const brandChroma = brandContract.chroma as [string, string, string, string, string, string, string];

const semanticThemes = {
  dark: {
    surface: {
      canvas: "#0B0C0E",
      raised: "#131519",
      elevated: "#1B1E24",
      overlay: "rgba(11, 12, 14, 0.88)",
      chrome: "rgba(11, 12, 14, 0.78)",
      entry: "rgba(11, 12, 14, 0.82)",
      identity: "rgba(11, 12, 14, 0.72)",
      focus: "#282C34",
    },
    content: {
      primary: "#E7EAF0",
      secondary: "#8B93A3",
      muted: "#8B93A3",
      inverse: "#0B0C0E",
    },
    border: {
      decorative: "#2A2E37",
      control: "#666970",
      // Now-airing is a neutral tone step (TV 5b; #1909 Q3): amber is focus and the primary action
      // only, so the airing block takes the control border and the focus surface, with no hue.
      airing: "#666970",
    },
    state: {
      live: "#E85A5F",
      success: "#3DD68C",
      warning: "#F5D90A",
      danger: "#E85A5F",
      info: "#4CC9E8",
    },
    stateSurface: {
      live: "#331D21",
      success: "#19322A",
      warning: "#353217",
      danger: "#331D21",
      info: "#1C3038",
      airing: "#282C34",
    },
    action: {
      primary: "#FFB020",
      secondary: "#E7EAF0",
      focus: "#FFB020",
      disabled: "#5A6170",
    },
    // A brand accent that is neither a state nor the primary action: the suggest/AI magenta's
    // own accessible stop, same in both themes (the brand mark's chroma is theme-invariant, same
    // as `guide.onAir` below).
    accent: {
      suggest: "#DC5BAC",
    },
    // The monogram ident's letters, one per hue (#3011): each clears 4.5:1 on its own 14% wash over
    // the canvas, a raised card and the focus surface a selected guide row puts it on. Where the
    // hue's text stop missed on focus, the letters take the nearest lighter step (maintainer,
    // 2026-11-09): magenta #DC5BAC 3.6:1, red #E85A5F 3.5:1.
    identLetter: {
      tune: "#4CC9E8",
      suggest: "#E37CBD",
      signal: "#FFB020",
      onair: "#ED7B7F",
    },
    artwork: {
      placeholder: "#1B1E24",
      scrim: "rgba(11, 12, 14, 0.66)",
      // Over a photograph, so the same in both themes (#1659 web mock cards): the foot of the
      // overlay card's gradient, and the progress track drawn across a still.
      scrimStrong: "rgba(11, 12, 14, 0.92)",
      progressTrack: "rgba(255, 255, 255, 0.15)",
    },
    // The web mock's time grid (#1659): the row rule, an ordinary block's left accent, the dashed
    // pending block, the break wash and the per-clip segments inside it.
    guide: {
      rowRule: "rgba(42, 46, 55, 0.6)",
      blockAccent: "#3A3F49",
      pendingFill: "rgba(76, 201, 232, 0.08)",
      pendingBorder: "rgba(76, 201, 232, 0.4)",
      breakFill: "rgba(255, 176, 32, 0.3)",
      clipSignal: "rgba(255, 176, 32, 0.4)",
      clipTune: "rgba(76, 201, 232, 0.4)",
      clipSuggest: "rgba(214, 64, 159, 0.4)",
      clipPlain: "#2A2E37",
      // The rail's on-air and reconciling dots (the brand onair red, and the lighter queued amber).
      onAir: "#E5484D",
      reconciling: "#FFC14D",
    },
    // A channel's era colour, drawn only on its number block (#2767). The decade picks the first
    // colour of the on-screen logo's era palette (internal/playout/watermark/design.go eraPalettes),
    // from the brand chroma, so it is the same in both themes; a decade with no palette is neutral.
    // The number on it is the brand ground.
    decade: {
      "1970": brandChroma[1],
      "1980": brandChroma[5],
      "1990": brandChroma[3],
      ink: brandContract.ground,
    },
  },
  light: {
    surface: {
      canvas: "#F7F8FA",
      raised: "#FFFFFF",
      elevated: "#E7EAF0",
      overlay: "rgba(255, 255, 255, 0.9)",
      chrome: "rgba(255, 255, 255, 0.78)",
      entry: "rgba(255, 255, 255, 0.82)",
      identity: "rgba(255, 255, 255, 0.72)",
      // Selection is neutral (#1909, #1953): one grey step past `elevated`, a secondary action's
      // resting fill, as dark's focus is one step past its own. The same step as `stateSurface.airing`.
      focus: "#D2D6DE",
    },
    content: {
      primary: "#17191D",
      secondary: "#515866",
      muted: "#69717F",
      inverse: "#FFFFFF",
    },
    border: {
      decorative: "#D2D6DE",
      control: "#747C8B",
      // As in dark: the airing block takes the control border and the focus surface (`#D2D6DE`),
      // one grey step below `elevated`.
      airing: "#747C8B",
    },
    state: {
      live: "#A5272C",
      success: "#116B45",
      warning: "#655500",
      danger: "#A5272C",
      info: "#08657A",
    },
    stateSurface: {
      live: "#F2DFDF",
      success: "#DBE9E3",
      warning: "#E8E6D9",
      danger: "#F2DFDF",
      info: "#DAE8EB",
      airing: "#D2D6DE",
    },
    action: {
      primary: "#795000",
      secondary: "#2A2E37",
      focus: "#8A5700",
      disabled: "#8B93A3",
    },
    // The magenta's own text stop on white: #DC5BAC is 3.4:1 there, this is 6.7:1 (and 5.5:1 on
    // the suggest badge's 15% wash). Amber's light text stop is `action.primary` (#795000, 7.1:1).
    accent: {
      suggest: "#A8246F",
    },
    // As in dark, the nearest darker step where the hue's text stop missed 4.5:1 on the focus
    // surface: magenta #A8246F 3.9:1, teal #08657A 4.3:1, red #A5272C 4.2:1.
    identLetter: {
      tune: "#086276",
      suggest: "#8F1F5E",
      signal: "#795000",
      onair: "#9E252A",
    },
    artwork: {
      placeholder: "#E7EAF0",
      scrim: "rgba(11, 12, 14, 0.58)",
      scrimStrong: "rgba(11, 12, 14, 0.92)",
      progressTrack: "rgba(255, 255, 255, 0.15)",
    },
    // The light guide has no mock: each tint is the light theme's own hue at the dark mock's alpha.
    guide: {
      rowRule: "rgba(210, 214, 222, 0.6)",
      blockAccent: "#747C8B",
      pendingFill: "rgba(8, 101, 122, 0.08)",
      pendingBorder: "rgba(8, 101, 122, 0.4)",
      breakFill: "rgba(121, 80, 0, 0.3)",
      clipSignal: "rgba(121, 80, 0, 0.4)",
      clipTune: "rgba(8, 101, 122, 0.4)",
      clipSuggest: "rgba(214, 64, 159, 0.4)",
      clipPlain: "#D2D6DE",
      onAir: "#E5484D",
      reconciling: "#FFC14D",
    },
    decade: {
      "1970": brandChroma[1],
      "1980": brandChroma[5],
      "1990": brandChroma[3],
      ink: brandContract.ground,
    },
  },
} as const;

const semanticColors = {
  brand: {
    signal: brandChroma[0],
    ground: brandContract.ground,
    foreground: brandContract.foreground,
    chroma: brandChroma,
  },
  ...semanticThemes.dark,
} as const;

const semanticSpace = {
  screen: 32,
  section: 24,
  control: 12,
  inline: 8,
} as const;

const semanticRadius = {
  control: 10,
  card: 16,
  // The web mock's compact card (#1659): Home's Watching now and the Guide's On now.
  cardCompact: 12,
  overlay: 24,
  round: 999,
} as const;

const semanticMotion = {
  instant: 0,
  focus: 140,
  overlay: 220,
  tune: 280,
} as const;

const semanticTargets = {
  pointer: 40,
  touch: 48,
  tv: 64,
} as const;

const iconography = {
  size: {
    compact: 16,
    default: 20,
    // The Material bottom bar's glyph inside its pill (#1659 native mock 5f).
    navigation: 22,
    control: 24,
    touch: 28,
    tv: 36,
    // The player's tools (§9.4): one size and one weight, lighter than the app's glyphs.
    player: 18,
  },
  strokeWidth: 2,
  playerStrokeWidth: 1.75,
} as const;

// Player chrome (frontend-design §9.1). It always sits over a moving picture, so it is the same in
// both themes. It is flat (#2769): solid surfaces, one hairline and one bottom-up scrim; no
// gradients, inset highlights, drop shadows or backdrop blur. The amber and red are the brand's
// signal and on-air; the break tint is the tune cyan.
const playerChrome = {
  // Solid surfaces and one hairline: nothing is translucent, lit or blurred.
  surface: semanticThemes.dark.surface.canvas,
  // The band, its film strip and preview cards, and what the keys open (§9.1, variant J).
  band: semanticThemes.dark.surface.raised,
  surfaceRaised: semanticThemes.dark.surface.elevated,
  surfaceHover: semanticThemes.dark.surface.focus,
  hairline: "rgba(255, 255, 255, 0.16)",
  // The one soft edge: behind the picture's lower planes (the bottom-up scrim), and behind an open sheet.
  scrimFoot: "rgba(0, 0, 0, 0.85)",
  scrimMid: "rgba(0, 0, 0, 0.45)",
  dim: "rgba(0, 0, 0, 0.60)",
  signal: "#FFB020",
  signalWash: "rgba(255, 176, 32, 0.22)",
  signalPast: "rgba(255, 176, 32, 0.12)",
  signalPastRing: "rgba(255, 176, 32, 0.25)",
  live: "#E85A5F",
  breakTint: "rgba(76, 201, 232, 0.15)",
  breakRing: "rgba(76, 201, 232, 0.35)",
  breakAccent: "#4CC9E8",
  focusRing: "rgba(255, 255, 255, 0.9)",
  // A future segment, further away: dimmer and less saturated (§9.2).
  futureFilter: "brightness(0.72) saturate(0.7)",
  litFilter: "brightness(1.1)",
  track: "rgba(255, 255, 255, 0.15)",
  disc: "#FFFFFF",
  discInk: "#0B0C0E",
  text: "#F4F6FA",
  textSecondary: "#B8BECA",
  textMuted: "#8B93A3",
  tooltip: "rgba(11, 12, 14, 0.95)",
  // A panel docked inside the frame (`DockedPanel`): the canvas at 95%, so the picture shows faintly.
  panel: "rgba(11, 12, 14, 0.95)",
  // Behind a subtitle line: near-black, so the line reads on any picture (§9.5).
  caption: "rgba(0, 0, 0, 0.78)",
} as const;

const typography = {
  family: {
    // Native has no variable font: each app embeds one static file per weight the native mock sets
    // (Geist 400–700, Geist Mono 400–600), listed once in native-fonts.json for the apps' font plugin
    // (web/scripts/with-loomarr-fonts.cjs), and `nativeFontFace` names them.
    body: {
      web: "'Geist Variable', Geist, ui-sans-serif, system-ui, sans-serif",
      native: nativeFonts.body,
    },
    data: {
      web: "'Geist Mono Variable', 'Geist Mono', ui-monospace, monospace",
      native: nativeFonts.data,
    },
  },
  // Pointer is the web's density, so its body is the web scale's body (`--text-base` 14 at
  // `--leading-body` 1.5; #1909 Q9), and shared components match the Tailwind page around them.
  // `data` is body in mono, as at every density.
  pointer: {
    // The five type roles (#2788): Display, Heading, Body, Label and Mono, sized per density. Pointer
    // takes the design review's 32/600, 18/600, 14/400 and 12/500; Mono (channel numbers and clocks
    // only) is Label's size. Touch and TV have no decided sizes yet, so each keeps its current value
    // and the remaining roles fold into these five.
    display: { size: 32, lineHeight: 36, weight: "600" },
    heading: { size: 18, lineHeight: 24, weight: "600" },
    mono: { size: 12, lineHeight: 16, weight: "400" },
    title: { size: 24, lineHeight: 30, weight: "650" },
    body: { size: 14, lineHeight: 21, weight: "400" },
    compact: { size: 13, lineHeight: 20, weight: "400" },
    caption: { size: 13, lineHeight: 18, weight: "400" },
    headline: { size: 20, lineHeight: 30, weight: "400" },
    label: { size: 12, lineHeight: 16, weight: "500" },
    section: { size: 14, lineHeight: 20, weight: "400" },
    metadata: { size: 12, lineHeight: 18, weight: "500" },
    // A form field's help or validation line (FieldHint, #970 step 45): the Web forms' existing
    // 12px/16px regular measure. Touch and TV take cardMeta's size at each density, the nearest
    // existing role, so no size is invented for a density nobody has mocked.
    hint: { size: 12, lineHeight: 16, weight: "400" },
    time: { size: 13, lineHeight: 18, weight: "550" },
    data: { size: 14, lineHeight: 21, weight: "400" },
    reading: { size: 16, lineHeight: 24, weight: "400" },
    channelNumber: { size: 28, lineHeight: 32, weight: "650" },
    // The player band's channel block (§9.2, variant J): the number fills a 96px block.
    bandNumber: { size: 48, lineHeight: 52, weight: "650" },
    code: { size: 36, lineHeight: 43, weight: "400" },
    // The compact card (#1659 web mock), a pointer-density design. At touch and TV each role
    // takes its nearest existing role's size (title → label, label → caption, meta and channel
    // number, time and initials → metadata), so no size is invented for a density nobody has mocked.
    cardTitle: { size: 15, lineHeight: 20, weight: "600" },
    cardLabel: { size: 13, lineHeight: 18, weight: "500" },
    cardMeta: { size: 12, lineHeight: 16, weight: "400" },
    cardChannelNumber: { size: 12, lineHeight: 16, weight: "600" },
    cardTime: { size: 12, lineHeight: 16, weight: "400" },
    // The web mock's time grid (#1659): the channel number. Touch and TV take headline's size.
    guideNumber: { size: 20, lineHeight: 20, weight: "600" },
    // A phone tab's large title and a phone Guide cell's title (#1659 native mock 5d). Neither has a
    // pointer mock, so pointer takes the nearest existing role (title, label).
    screenTitle: { size: 24, lineHeight: 30, weight: "650" },
    guideCell: { size: 11, lineHeight: 14, weight: "400" },
    // A subtitle line over the picture (§9.5, variant J): 24 on desktop, 15 on a phone on its side,
    // 18 on a television. An upright phone's picture is a strip, so its line is smaller still (13)
    // at every density, the one place it is drawn.
    subtitle: { size: 24, lineHeight: 31, weight: "500" },
    subtitleUpright: { size: 13, lineHeight: 17, weight: "500" },
  },
  touch: {
    display: { size: 38, lineHeight: 42, weight: "700" },
    heading: { size: 24, lineHeight: 30, weight: "650" },
    mono: { size: 13, lineHeight: 18, weight: "400" },
    title: { size: 24, lineHeight: 30, weight: "650" },
    body: { size: 17, lineHeight: 25, weight: "400" },
    compact: { size: 14, lineHeight: 21, weight: "400" },
    caption: { size: 14, lineHeight: 20, weight: "400" },
    headline: { size: 22, lineHeight: 33, weight: "400" },
    label: { size: 15, lineHeight: 21, weight: "600" },
    section: { size: 15, lineHeight: 22, weight: "400" },
    metadata: { size: 13, lineHeight: 18, weight: "500" },
    hint: { size: 13, lineHeight: 18, weight: "400" },
    time: { size: 14, lineHeight: 20, weight: "550" },
    data: { size: 17, lineHeight: 25, weight: "400" },
    reading: { size: 17, lineHeight: 25, weight: "400" },
    channelNumber: { size: 30, lineHeight: 34, weight: "650" },
    // A phone's 56px block: three digits step down to `guideNumber` to keep a margin.
    bandNumber: { size: 30, lineHeight: 34, weight: "650" },
    code: { size: 40, lineHeight: 48, weight: "400" },
    cardTitle: { size: 15, lineHeight: 21, weight: "600" },
    cardLabel: { size: 14, lineHeight: 20, weight: "500" },
    cardMeta: { size: 13, lineHeight: 18, weight: "400" },
    cardChannelNumber: { size: 13, lineHeight: 18, weight: "600" },
    cardTime: { size: 13, lineHeight: 18, weight: "400" },
    guideNumber: { size: 22, lineHeight: 22, weight: "600" },
    // 5d: the "Guide" title at 30/700 (-0.02em), and the grid's programme titles in Geist 13/500.
    screenTitle: { size: 30, lineHeight: 36, weight: "700" },
    guideCell: { size: 13, lineHeight: 18, weight: "500" },
    subtitle: { size: 15, lineHeight: 20, weight: "500" },
    subtitleUpright: { size: 13, lineHeight: 17, weight: "500" },
  },
  tv: {
    display: { size: 54, lineHeight: 60, weight: "700" },
    heading: { size: 32, lineHeight: 39, weight: "650" },
    mono: { size: 16, lineHeight: 22, weight: "400" },
    title: { size: 32, lineHeight: 39, weight: "650" },
    body: { size: 22, lineHeight: 31, weight: "400" },
    compact: { size: 20, lineHeight: 30, weight: "400" },
    caption: { size: 17, lineHeight: 26, weight: "400" },
    headline: { size: 30, lineHeight: 45, weight: "400" },
    label: { size: 18, lineHeight: 25, weight: "600" },
    section: { size: 20, lineHeight: 30, weight: "400" },
    metadata: { size: 16, lineHeight: 22, weight: "500" },
    hint: { size: 16, lineHeight: 22, weight: "400" },
    time: { size: 18, lineHeight: 24, weight: "550" },
    data: { size: 24, lineHeight: 30, weight: "400" },
    reading: { size: 24, lineHeight: 36, weight: "400" },
    channelNumber: { size: 38, lineHeight: 43, weight: "650" },
    bandNumber: { size: 48, lineHeight: 52, weight: "650" },
    code: { size: 52, lineHeight: 62, weight: "400" },
    cardTitle: { size: 18, lineHeight: 25, weight: "600" },
    cardLabel: { size: 17, lineHeight: 26, weight: "500" },
    cardMeta: { size: 16, lineHeight: 22, weight: "400" },
    cardChannelNumber: { size: 16, lineHeight: 22, weight: "600" },
    cardTime: { size: 16, lineHeight: 22, weight: "400" },
    guideNumber: { size: 30, lineHeight: 30, weight: "600" },
    // 5b's TV Guide title is 30/600; the TV grid has no cell-title role of its own.
    screenTitle: { size: 30, lineHeight: 36, weight: "600" },
    guideCell: { size: 16, lineHeight: 22, weight: "400" },
    subtitle: { size: 18, lineHeight: 23, weight: "500" },
    subtitleUpright: { size: 13, lineHeight: 17, weight: "500" },
  },
} as const;

type Density = keyof typeof semanticTargets;
type TextRole = keyof (typeof typography)["pointer"];
type FontFamilyRole = keyof (typeof typography)["family"];

/**
 * The embedded weight a requested weight draws with, by CSS font matching: a weight from 400 to 500
 * tries itself up to 500, then lighter, then heavier; a lighter one tries lighter first, a heavier one
 * heavier first. So the type scale's 550 draws SemiBold and 650 draws Bold, as a browser would draw
 * them from these static files.
 */
const nearestEmbeddedWeight = (embedded: readonly number[], requested: number): number => {
  const sorted = [...embedded].sort((a, b) => a - b);
  if (sorted.includes(requested)) return requested;
  const lighter = sorted.filter((weight) => weight < requested).reverse();
  const heavier = sorted.filter((weight) => weight > requested);
  const order =
    requested < 400
      ? [...lighter, ...heavier]
      : requested <= 500
        ? [
            ...heavier.filter((weight) => weight <= 500),
            ...lighter,
            ...heavier.filter((weight) => weight > 500),
          ]
        : [...heavier, ...lighter];
  return order[0] ?? sorted[0] ?? requested;
};

/**
 * The native font name for a family role at a weight: iOS registers each file by its PostScript name
 * (`Geist-SemiBold`), Android by its file name (`Geist_600SemiBold`). Omitted, the weight is 400.
 */
const nativeFontFace = (role: FontFamilyRole, os: "android" | "ios", weight: number | string = 400) => {
  const { file, weights } = typography.family[role].native;
  const requested = weight === "bold" ? 700 : weight === "normal" ? 400 : Number(weight) || 400;
  const embedded = nearestEmbeddedWeight(Object.keys(weights).map(Number), requested);
  const style = (weights as Record<string, string>)[String(embedded)];
  return os === "android" ? `${file}_${embedded}${style}` : `${file}-${style}`;
};

export type { Density, FontFamilyRole, TextRole };
export {
  brandChroma,
  brandContract,
  iconography,
  nativeFontFace,
  playerChrome,
  semanticColors,
  semanticMotion,
  semanticRadius,
  semanticSpace,
  semanticTargets,
  semanticThemes,
  typography,
};
