import { createFont, createTamagui, createTokens, type GenericFont, isWeb } from "@tamagui/core";
import { Platform } from "react-native";

import {
  type FontFamilyRole,
  nativeFontFace,
  semanticRadius,
  semanticSpace,
  semanticTargets,
  semanticThemes,
  typography,
} from "../tokens";

const themeColors = (mode: keyof typeof semanticThemes) => ({
  surfaceCanvas: semanticThemes[mode].surface.canvas,
  surfaceRaised: semanticThemes[mode].surface.raised,
  surfaceElevated: semanticThemes[mode].surface.elevated,
  surfaceOverlay: semanticThemes[mode].surface.overlay,
  surfaceChrome: semanticThemes[mode].surface.chrome,
  surfaceEntry: semanticThemes[mode].surface.entry,
  surfaceIdentity: semanticThemes[mode].surface.identity,
  surfaceFocus: semanticThemes[mode].surface.focus,
  contentPrimary: semanticThemes[mode].content.primary,
  contentSecondary: semanticThemes[mode].content.secondary,
  contentMuted: semanticThemes[mode].content.muted,
  contentInverse: semanticThemes[mode].content.inverse,
  borderDecorative: semanticThemes[mode].border.decorative,
  borderControl: semanticThemes[mode].border.control,
  borderAiring: semanticThemes[mode].border.airing,
  stateLive: semanticThemes[mode].state.live,
  stateSuccess: semanticThemes[mode].state.success,
  stateWarning: semanticThemes[mode].state.warning,
  stateDanger: semanticThemes[mode].state.danger,
  stateInfo: semanticThemes[mode].state.info,
  stateLiveSurface: semanticThemes[mode].stateSurface.live,
  stateSuccessSurface: semanticThemes[mode].stateSurface.success,
  stateWarningSurface: semanticThemes[mode].stateSurface.warning,
  stateDangerSurface: semanticThemes[mode].stateSurface.danger,
  stateInfoSurface: semanticThemes[mode].stateSurface.info,
  stateAiringSurface: semanticThemes[mode].stateSurface.airing,
  actionPrimary: semanticThemes[mode].action.primary,
  actionSecondary: semanticThemes[mode].action.secondary,
  actionFocus: semanticThemes[mode].action.focus,
  actionDisabled: semanticThemes[mode].action.disabled,
  accentSuggest: semanticThemes[mode].accent.suggest,
  artworkPlaceholder: semanticThemes[mode].artwork.placeholder,
  artworkScrim: semanticThemes[mode].artwork.scrim,
  artworkScrimStrong: semanticThemes[mode].artwork.scrimStrong,
  artworkProgressTrack: semanticThemes[mode].artwork.progressTrack,
  guideRowRule: semanticThemes[mode].guide.rowRule,
  guideBlockAccent: semanticThemes[mode].guide.blockAccent,
  guidePendingFill: semanticThemes[mode].guide.pendingFill,
  guidePendingBorder: semanticThemes[mode].guide.pendingBorder,
  guideBreakFill: semanticThemes[mode].guide.breakFill,
  guideClipSignal: semanticThemes[mode].guide.clipSignal,
  guideClipTune: semanticThemes[mode].guide.clipTune,
  guideClipSuggest: semanticThemes[mode].guide.clipSuggest,
  guideClipPlain: semanticThemes[mode].guide.clipPlain,
  guideOnAir: semanticThemes[mode].guide.onAir,
  guideReconciling: semanticThemes[mode].guide.reconciling,
});

const tamaguiTheme = (mode: keyof typeof semanticThemes) => {
  const colors = themeColors(mode);

  return {
    ...colors,
    // Theme entries are emitted as CSS custom properties on web. Tamagui does not
    // resolve one theme entry through another, so aliases here must be concrete
    // values rather than token names such as "surfaceCanvas". An unresolved alias also
    // poisons legacy declarations that consume the same custom-property name (for
    // example Tailwind's ring-offset shadow reads --background).
    background: colors.surfaceCanvas,
    backgroundHover: colors.surfaceRaised,
    backgroundFocus: colors.surfaceFocus,
    backgroundPress: colors.surfaceElevated,
    color: colors.contentPrimary,
    colorHover: colors.contentPrimary,
    colorFocus: colors.contentPrimary,
    colorPress: colors.contentPrimary,
    borderColor: colors.surfaceElevated,
    borderColorFocus: colors.actionFocus,
    outlineColor: colors.actionFocus,
  };
};

// Colours live only in the themes. A colour token of the same name would win over the theme key
// in Tamagui 3 ("contentPrimary" resolves tokens first) and pin every surface to the dark value.
const tokens = createTokens({
  radius: {
    ...semanticRadius,
  },
  size: {
    ...semanticTargets,
  },
  space: {
    ...semanticSpace,
  },
  zIndex: {
    base: 0,
    overlay: 10,
  },
});

const nativeOs = Platform.OS === "android" ? "android" : "ios";

// On native, Tamagui swaps a text's family for the face its weight names, so each weight draws its
// own embedded file. The type scale's 550 and 650 are not Tamagui `FontWeightValues`, so the record
// is widened; Tamagui keeps the extra keys.
const nativeFaces = (role: FontFamilyRole) =>
  Object.fromEntries(
    ["100", "200", "300", "400", "500", "550", "600", "650", "700", "800", "900", "normal", "bold"].map(
      (weight) => [weight, { normal: nativeFontFace(role, nativeOs, weight) }],
    ),
  ) as GenericFont["face"];

const bodyFont = createFont({
  family: isWeb ? typography.family.body.web : nativeFontFace("body", nativeOs),
  ...(isWeb ? {} : { face: nativeFaces("body") }),
  size: Object.fromEntries(
    Object.entries(typography.pointer).map(([role, value]) => [role, value.size]),
  ) as Record<keyof (typeof typography)["pointer"], number>,
  lineHeight: Object.fromEntries(
    Object.entries(typography.pointer).map(([role, value]) => [role, value.lineHeight]),
  ) as Record<keyof (typeof typography)["pointer"], number>,
  weight: {
    regular: "400",
    medium: "500",
    semibold: "600",
    bold: "700",
  },
  letterSpacing: {
    display: -0.8,
    title: -0.25,
    body: 0,
    compact: 0,
    caption: 0,
    headline: 0,
    label: 0,
    section: 2,
    metadata: 0.2,
    time: 0,
    data: 0,
    reading: 0,
    channelNumber: -0.4,
    code: 0,
  },
});

const dataFont = createFont({
  ...bodyFont,
  family: isWeb ? typography.family.data.web : nativeFontFace("data", nativeOs),
  ...(isWeb ? {} : { face: nativeFaces("data") }),
});

const loomarrConfig = createTamagui({
  fonts: {
    body: bodyFont,
    data: dataFont,
  },
  themes: {
    dark: tamaguiTheme("dark"),
    light: tamaguiTheme("light"),
  },
  tokens,
  settings: {
    defaultFont: "body",
    onlyAllowShorthands: false,
  },
});

type LoomarrConfig = typeof loomarrConfig;

declare module "@tamagui/core" {
  interface TamaguiCustomConfig extends LoomarrConfig {}
}

export type { LoomarrConfig };
export { loomarrConfig };
