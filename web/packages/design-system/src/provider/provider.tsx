import { TamaguiProvider } from "@tamagui/core";
import type { PropsWithChildren } from "react";
import { type ColorSchemeName, useColorScheme } from "react-native";

import { loomarrConfig } from "../config";
import { type ViewportInsets, ViewportInsetsProvider } from "../viewport";

type LoomarrTheme = "dark" | "light";
type LoomarrProviderProps = PropsWithChildren<{
  insets?: ViewportInsets;
  theme?: LoomarrTheme | "system";
}>;

const resolveLoomarrTheme = (
  theme: LoomarrProviderProps["theme"] = "dark",
  systemTheme?: ColorSchemeName | null,
): LoomarrTheme => (theme === "system" ? (systemTheme === "light" ? "light" : "dark") : theme);

const LoomarrProvider = ({ children, insets, theme }: LoomarrProviderProps) => {
  const systemTheme = useColorScheme();
  const resolvedTheme = resolveLoomarrTheme(theme, systemTheme);

  return (
    <TamaguiProvider config={loomarrConfig} defaultTheme={resolvedTheme}>
      <ViewportInsetsProvider insets={insets}>{children}</ViewportInsetsProvider>
    </TamaguiProvider>
  );
};

export type { LoomarrProviderProps, LoomarrTheme };
export { LoomarrProvider, resolveLoomarrTheme };
