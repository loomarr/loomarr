import { createFont, createTamagui, createTokens, isWeb } from "@tamagui/core";

import {
  semanticColors,
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
  surfaceFocus: semanticThemes[mode].surface.focus,
  contentPrimary: semanticThemes[mode].content.primary,
  contentSecondary: semanticThemes[mode].content.secondary,
  contentMuted: semanticThemes[mode].content.muted,
  contentInverse: semanticThemes[mode].content.inverse,
  borderDecorative: semanticThemes[mode].border.decorative,
  borderControl: semanticThemes[mode].border.control,
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
  actionPrimary: semanticThemes[mode].action.primary,
  actionSecondary: semanticThemes[mode].action.secondary,
  actionFocus: semanticThemes[mode].action.focus,
  actionDisabled: semanticThemes[mode].action.disabled,
  artworkPlaceholder: semanticThemes[mode].artwork.placeholder,
  artworkScrim: semanticThemes[mode].artwork.scrim,
});

const tokens = createTokens({
  color: {
    surfaceCanvas: semanticColors.surface.canvas,
    surfaceRaised: semanticColors.surface.raised,
    surfaceElevated: semanticColors.surface.elevated,
    surfaceOverlay: semanticColors.surface.overlay,
    surfaceFocus: semanticColors.surface.focus,
    contentPrimary: semanticColors.content.primary,
    contentSecondary: semanticColors.content.secondary,
    contentMuted: semanticColors.content.muted,
    contentInverse: semanticColors.content.inverse,
    borderDecorative: semanticColors.border.decorative,
    borderControl: semanticColors.border.control,
    stateLive: semanticColors.state.live,
    stateSuccess: semanticColors.state.success,
    stateWarning: semanticColors.state.warning,
    stateDanger: semanticColors.state.danger,
    stateInfo: semanticColors.state.info,
    stateLiveSurface: semanticColors.stateSurface.live,
    stateSuccessSurface: semanticColors.stateSurface.success,
    stateWarningSurface: semanticColors.stateSurface.warning,
    stateDangerSurface: semanticColors.stateSurface.danger,
    stateInfoSurface: semanticColors.stateSurface.info,
    actionPrimary: semanticColors.action.primary,
    actionSecondary: semanticColors.action.secondary,
    actionFocus: semanticColors.action.focus,
    actionDisabled: semanticColors.action.disabled,
    artworkPlaceholder: semanticColors.artwork.placeholder,
    artworkScrim: semanticColors.artwork.scrim,
    transparent: "transparent",
  },
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

const bodyFont = createFont({
  family: isWeb ? typography.family.body.web : typography.family.body.native,
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
    label: 0,
    metadata: 0.2,
    time: 0,
    channelNumber: -0.4,
  },
});

const dataFont = createFont({
  ...bodyFont,
  family: isWeb ? typography.family.data.web : typography.family.data.native,
});

const loomarrConfig = createTamagui({
  defaultTheme: "dark",
  defaultFont: "body",
  fonts: {
    body: bodyFont,
    data: dataFont,
  },
  themes: {
    dark: {
      ...themeColors("dark"),
      background: "$surfaceCanvas",
      backgroundHover: "$surfaceRaised",
      backgroundFocus: "$surfaceFocus",
      backgroundPress: "$surfaceElevated",
      color: "$contentPrimary",
      colorHover: "$contentPrimary",
      colorFocus: "$contentPrimary",
      colorPress: "$contentPrimary",
      borderColor: "$surfaceElevated",
      borderColorFocus: "$actionFocus",
      outlineColor: "$actionFocus",
    },
    light: {
      ...themeColors("light"),
      background: "$surfaceCanvas",
      backgroundHover: "$surfaceRaised",
      backgroundFocus: "$surfaceFocus",
      backgroundPress: "$surfaceElevated",
      color: "$contentPrimary",
      colorHover: "$contentPrimary",
      colorFocus: "$contentPrimary",
      colorPress: "$contentPrimary",
      borderColor: "$surfaceElevated",
      borderColorFocus: "$actionFocus",
      outlineColor: "$actionFocus",
    },
  },
  tokens,
  settings: {
    onlyAllowShorthands: false,
  },
});

type LoomarrConfig = typeof loomarrConfig;

declare module "@tamagui/core" {
  interface TamaguiCustomConfig extends LoomarrConfig {}
}

export type { LoomarrConfig };
export { loomarrConfig, tokens };
