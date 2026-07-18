import type { StorybookConfig } from "@storybook/react-vite";

// Storybook 10 (react-vite) — the component workshop AND the offline gallery the visual
// suite snapshots (frontend-design §5). Stories are co-located with their components
// (folder-per-component). a11y runs live in the workshop via addon-a11y; the CI gate is
// a Playwright pass over `storybook build` output (`storybook-static`). Chromatic rejected.
const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: ["@storybook/addon-a11y"],
  framework: { name: "@storybook/react-vite", options: {} },
  core: { disableTelemetry: true },
};

export default config;
