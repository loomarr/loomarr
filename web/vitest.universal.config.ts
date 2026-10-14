import { fileURLToPath } from "node:url";

const reactNativeWeb = fileURLToPath(
  new URL("./node_modules/react-native-web/dist/cjs/index.js", import.meta.url),
);
const reactNativeSvgWeb = fileURLToPath(
  new URL("./node_modules/react-native-svg/lib/module/elements.web.js", import.meta.url),
);

export default {
  resolve: {
    alias: [
      { find: /^react-native$/, replacement: reactNativeWeb },
      { find: /^react-native-svg$/, replacement: reactNativeSvgWeb },
    ],
    dedupe: ["react", "react-native"],
    extensions: [".web.mjs", ".web.js", ".web.ts", ".web.tsx", ".mjs", ".js", ".ts", ".tsx", ".json"],
  },
  ssr: { noExternal: ["lucide-react-native"] },
};
