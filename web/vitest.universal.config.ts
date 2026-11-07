import { fileURLToPath } from "node:url";

const reactNativeWeb = fileURLToPath(
  new URL("./node_modules/react-native-web/dist/cjs/index.js", import.meta.url),
);
const reactNativeSvgWeb = fileURLToPath(
  new URL("./node_modules/react-native-svg/lib/module/elements.web.js", import.meta.url),
);
// react-native-svg's web build imports React Native's asset registry package, which React Native
// 0.88 no longer installs (the registry moved inside react-native). On the web the registry is
// react-native-web's.
const reactNativeWebAssetRegistry = fileURLToPath(
  new URL("./node_modules/react-native-web/dist/cjs/modules/AssetRegistry/index.js", import.meta.url),
);

export default {
  resolve: {
    alias: [
      { find: /^react-native$/, replacement: reactNativeWeb },
      { find: /^react-native-svg$/, replacement: reactNativeSvgWeb },
      { find: /^@react-native\/assets-registry\/registry$/, replacement: reactNativeWebAssetRegistry },
    ],
    dedupe: ["react", "react-native"],
    extensions: [".web.mjs", ".web.js", ".web.ts", ".web.tsx", ".mjs", ".js", ".ts", ".tsx", ".json"],
  },
  ssr: { noExternal: ["lucide-react-native"] },
};
