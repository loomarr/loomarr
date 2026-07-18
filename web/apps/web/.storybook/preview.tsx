import type { Preview } from "@storybook/react-vite";
// Same imports as the app entry (main.tsx) — self-hosted Geist (§2.2) + the Test Card
// theme — so stories render in the real design system, offline and deterministic. The
// `body` rule in styles.css paints the dark (§2.1) canvas inside the story iframe.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "../src/styles.css";

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
};

export default preview;
