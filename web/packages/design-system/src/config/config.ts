import { createTamagui, createTokens } from "@tamagui/core";

const tokens = createTokens({
  color: {
    surfaceCanvas: "#080b12",
    surfaceRaised: "#151b27",
    surfaceFocus: "#28354d",
    contentPrimary: "#f5f7fb",
    contentSecondary: "#aeb8c8",
    actionFocus: "#8eb7ff",
  },
  radius: {
    control: 10,
    card: 16,
    overlay: 22,
  },
  size: {
    control: 44,
    touch: 48,
    tv: 64,
  },
  space: {
    inline: 8,
    control: 12,
    section: 24,
    screen: 32,
  },
  zIndex: {
    base: 0,
    overlay: 10,
  },
});

const loomarrConfig = createTamagui({
  defaultTheme: "dark",
  themes: {
    dark: {
      background: "$surfaceCanvas",
      backgroundHover: "$surfaceRaised",
      backgroundFocus: "$surfaceFocus",
      color: "$contentPrimary",
      colorHover: "$contentPrimary",
      colorFocus: "$contentPrimary",
      borderColor: "$surfaceFocus",
      outlineColor: "$actionFocus",
    },
  },
  tokens,
});

type LoomarrConfig = typeof loomarrConfig;

declare module "@tamagui/core" {
  interface TamaguiCustomConfig extends LoomarrConfig {}
}

export type { LoomarrConfig };
export { loomarrConfig };
