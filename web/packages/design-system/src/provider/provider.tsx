import { TamaguiProvider } from "@tamagui/core";
import type { PropsWithChildren } from "react";
import { type ColorSchemeName, useColorScheme } from "react-native";

import { loomarrConfig } from "../config";

type LoomarrTheme = "dark" | "light";
type LoomarrProviderProps = PropsWithChildren<{ theme?: LoomarrTheme | "system" }>;

const resolveLoomarrTheme = (
  theme: LoomarrProviderProps["theme"] = "dark",
  systemTheme?: ColorSchemeName | null,
): LoomarrTheme => (theme === "system" ? (systemTheme === "light" ? "light" : "dark") : theme);

const LoomarrProvider = ({ children, theme }: LoomarrProviderProps) => {
  const systemTheme = useColorScheme();
  const resolvedTheme = resolveLoomarrTheme(theme, systemTheme);

  return (
    <TamaguiProvider config={loomarrConfig} defaultTheme={resolvedTheme}>
      {children}
    </TamaguiProvider>
  );
};

export type { LoomarrProviderProps, LoomarrTheme };
export { LoomarrProvider, resolveLoomarrTheme };
