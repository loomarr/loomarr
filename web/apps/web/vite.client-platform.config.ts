import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const webReact = fileURLToPath(new URL("./node_modules/react", import.meta.url));
const webReactDOM = fileURLToPath(new URL("./node_modules/react-dom", import.meta.url));

export default defineConfig({
  plugins: [react()],
  // The shipping web app and Expo use different React patch versions. Linked universal
  // packages must resolve hooks through this adapter's React instance in the browser.
  resolve: {
    alias: {
      react: webReact,
      "react-dom": webReactDOM,
    },
    dedupe: ["react", "react-dom"],
  },
  // Vitest's server-render proof must transform the linked universal modules so the
  // adapter aliases above apply instead of loading their native React peer context.
  ssr: {
    noExternal: true,
  },
  build: {
    emptyOutDir: true,
    outDir: ".client-platform-proof",
    rollupOptions: {
      input: fileURLToPath(new URL("./client-platform-proof.html", import.meta.url)),
    },
  },
});
