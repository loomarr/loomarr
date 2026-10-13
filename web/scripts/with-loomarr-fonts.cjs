const path = require("node:path");
const { withPlugins } = require("@expo/config-plugins");

/**
 * Embed Geist and Geist Mono in a native app at build time through expo-font's config plugin, so text
 * never waits on a runtime font load. The weights come from the design system's native-fonts.json,
 * the same list its tokens name faces from, so an app can't embed a weight the type scale never
 * draws or miss one it does. Each file is `<package>/<weight><Style>/<File>_<weight><Style>.ttf`.
 */
function loomarrFontFiles(projectRoot) {
  const resolve = (request) => require.resolve(request, { paths: [projectRoot] });
  const families = require(resolve("@loomarr/design-system/native-fonts.json"));
  return Object.values(families).flatMap(({ file, package: fontPackage, weights }) => {
    const packageRoot = path.dirname(resolve(`${fontPackage}/package.json`));
    return Object.entries(weights).map(([weight, style]) =>
      path.relative(projectRoot, path.join(packageRoot, `${weight}${style}`, `${file}_${weight}${style}.ttf`)),
    );
  });
}

function withLoomarrFonts(config) {
  const projectRoot = config._internal?.projectRoot ?? process.cwd();
  return withPlugins(config, [["expo-font", { fonts: loomarrFontFiles(projectRoot) }]]);
}

module.exports = withLoomarrFonts;
module.exports.loomarrFontFiles = loomarrFontFiles;
