import { LoomarrProvider, resolveLoomarrTheme, semanticThemes } from "@loomarr/design-system";
import { withThemeFromJSXProvider } from "@storybook/addon-themes";
import type { Preview } from "@storybook/react-vite";
import type { PropsWithChildren } from "react";
import { useEffect } from "react";
import { useColorScheme } from "react-native";
// Same imports as the app entry (main.tsx) — self-hosted Geist (§2.2) + the Test Card
// theme — so stories render in the real design system, offline and deterministic. The
// `body` rule in styles.css paints the dark (§2.1) canvas inside the story iframe.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import { TooltipProvider } from "../src/components/ui";
import "../src/styles.css";

type WorkshopTheme = { mode: "dark" | "light" | "system" };

const WorkshopThemeProvider = ({ children, theme }: PropsWithChildren<{ theme: WorkshopTheme }>) => {
  const systemTheme = useColorScheme();
  const resolvedTheme = resolveLoomarrTheme(theme.mode, systemTheme);
  const colors = semanticThemes[resolvedTheme];

  useEffect(() => {
    document.documentElement.style.colorScheme = resolvedTheme;
    document.body.style.backgroundColor = colors.surface.canvas;
    document.body.style.color = colors.content.primary;
  }, [colors.content.primary, colors.surface.canvas, resolvedTheme]);

  return <LoomarrProvider theme={theme.mode}>{children}</LoomarrProvider>;
};

const preview: Preview = {
  parameters: {
    layout: "centered",
    // addon-a11y (axe) fails on serious/critical in the workshop panel (§5.3); the CI
    // gate re-runs axe over storybook-static via Playwright.
    a11y: { test: "error" },
    // The dark canvas comes from styles.css `body`, not the backgrounds addon — keep it
    // off so it can't paint a light background under the components.
    backgrounds: { disable: true },
    controls: { expanded: true },
  },
  // Mirror the app root (__root.tsx): a single TooltipProvider so any story with an
  // icon-only button's tooltip renders exactly as it does in the app — including the same
  // 300ms delay, which this decorator used to leave at the library default.
  decorators: [
    withThemeFromJSXProvider({
      Provider: WorkshopThemeProvider,
      defaultTheme: "dark",
      themes: {
        dark: { mode: "dark" },
        light: { mode: "light" },
        system: { mode: "system" },
      },
    }),
    (Story) => (
      <TooltipProvider delay={300}>
        <Story />
      </TooltipProvider>
    ),
  ],
};

export default preview;
