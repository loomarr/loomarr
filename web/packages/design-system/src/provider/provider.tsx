import { TamaguiProvider } from "@tamagui/core";
import type { PropsWithChildren } from "react";
import { useColorScheme } from "react-native";

import { loomarrConfig } from "../config";

type LoomarrTheme = "dark" | "light";
type LoomarrProviderProps = PropsWithChildren<{ theme?: LoomarrTheme | "system" }>;

const LoomarrProvider = ({ children, theme = "system" }: LoomarrProviderProps) => {
  const systemTheme = useColorScheme();
  const resolvedTheme: LoomarrTheme =
    theme === "system" ? (systemTheme === "light" ? "light" : "dark") : theme;

  return (
    <TamaguiProvider config={loomarrConfig} defaultTheme={resolvedTheme}>
      {children}
    </TamaguiProvider>
  );
};

export type { LoomarrProviderProps, LoomarrTheme };
export { LoomarrProvider };
