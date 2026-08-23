import { TamaguiProvider } from "@tamagui/core";
import type { PropsWithChildren } from "react";

import { loomarrConfig } from "../config";

const LoomarrProvider = ({ children }: PropsWithChildren) => (
  <TamaguiProvider config={loomarrConfig} defaultTheme="dark">
    {children}
  </TamaguiProvider>
);

export { LoomarrProvider };
