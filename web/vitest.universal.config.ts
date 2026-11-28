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

const universal = {
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

// The same aliases without the `.web.*` extensions, so an import resolves to the native `.tsx`
// adapter. React Native itself stays react-native-web: these tests drive the adapter in jsdom,
// not on a device.
const nativeAdapters = {
  ...universal,
  resolve: {
    ...universal.resolve,
    extensions: universal.resolve.extensions.filter((extension) => !extension.startsWith(".web.")),
  },
};

// A package with `tests/**/*.native.test.tsx` runs both adapters: `web` (everything else) and
// `native`. Each names its tests, so a file runs in exactly one project.
const withNativeProject = () => ({
  test: {
    projects: [
      {
        ...universal,
        test: {
          name: "web",
          include: ["{src,tests}/**/*.test.{ts,tsx}"],
          exclude: ["tests/**/*.native.test.tsx"],
        },
      },
      { ...nativeAdapters, test: { name: "native", include: ["tests/**/*.native.test.tsx"] } },
    ],
  },
});

export { withNativeProject };
export default universal;
