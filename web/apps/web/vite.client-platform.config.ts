import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const webReact = fileURLToPath(new URL("./node_modules/react", import.meta.url));
const webReactDOM = fileURLToPath(new URL("./node_modules/react-dom", import.meta.url));
const reactNativeWeb = fileURLToPath(new URL("./node_modules/react-native-web", import.meta.url));
const reactNativeSvgWeb = fileURLToPath(
  new URL("./node_modules/react-native-svg/lib/module/elements.web.js", import.meta.url),
);

export default defineConfig({
  plugins: [react()],
  // Linked universal packages must resolve React and native host elements through this
  // adapter's browser instances rather than pnpm's native peer context.
  resolve: {
    alias: [
      { find: /^react$/, replacement: webReact },
      { find: /^react-dom$/, replacement: webReactDOM },
      { find: /^react-native$/, replacement: reactNativeWeb },
      { find: /^react-native-svg$/, replacement: reactNativeSvgWeb },
    ],
    dedupe: ["react", "react-dom", "react-native"],
    extensions: [".web.mjs", ".web.js", ".web.ts", ".web.tsx", ".mjs", ".js", ".ts", ".tsx", ".json"],
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
