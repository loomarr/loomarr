import type { StorybookConfig } from "@storybook/react-vite";

// Storybook 10 (react-vite) — the component workshop AND the offline gallery the visual
// suite snapshots (frontend-design §5). Stories are co-located with their components
// (folder-per-component). a11y runs live in the workshop via addon-a11y; the CI gate is
// a Playwright pass over `storybook build` output (`storybook-static`). Chromatic rejected.
const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: ["@storybook/addon-a11y"],
  // ⚠ **`srcset` cannot be exercised with a data: URI, and this directory is why.** A base64 data
  // URI ALWAYS contains a comma (`data:image/png;base64,…`) and a comma is srcset's candidate
  // separator, so a data-URI candidate is unloadable — every UI/Image story rendered an <img> at
  // naturalWidth 0 and its baseline captured the ThumbHash placeholder rather than an image, green
  // forever (#210). Remote URLs are banned in visual stories because they race the snapshot, so
  // neither of the two obvious options works.
  //
  // These assets are same-origin (no race) and comma-free (loadable in srcset). They live HERE
  // rather than in `public/` deliberately: `public/` ships inside the app bundle, and these are
  // story fixtures, not product assets.
  staticDirs: ["./story-assets"],
  framework: { name: "@storybook/react-vite", options: {} },
  core: { disableTelemetry: true },
};

export default config;
