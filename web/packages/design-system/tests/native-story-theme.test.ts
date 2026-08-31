import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { describe, expect, it } from "vitest";

const nativeStories = fileURLToPath(new URL("../../../native-stories", import.meta.url));
const nativePreview = fileURLToPath(new URL("../../../.rnstorybook/preview.tsx", import.meta.url));
const storyFiles = readdirSync(nativeStories)
  .filter((name) => name.endsWith(".stories.tsx"))
  .sort();

describe("native Storybook theme contract", () => {
  it("defaults the on-device workshop to dark and resolves story globals through LoomarrProvider", () => {
    const preview = readFileSync(nativePreview, "utf8");
    expect(preview).toContain('initialGlobals: { theme: "dark" }');
    expect(preview).toContain('context.globals.theme === "light"');
  });

  it("forwards live native safe-area insets to every workshop story", () => {
    const preview = readFileSync(nativePreview, "utf8");
    expect(preview).toContain("<SafeAreaProvider>");
    expect(preview).toContain("const insets = useSafeAreaInsets()");
    expect(preview).toContain("<LoomarrProvider insets={insets}");
  });

  it("keeps a meaningful set of shared native modules in the workshop", () => {
    expect(storyFiles.length).toBeGreaterThanOrEqual(15);
  });

  it.each(storyFiles)("%s publishes an explicit light-mode state", (name) => {
    const source = readFileSync(`${nativeStories}/${name}`, "utf8");
    expect(source, `${name} needs a light story rendered through the native provider`).toMatch(
      /globals:\s*\{\s*theme:\s*"light"\s*\}/,
    );
  });

  it.each(storyFiles)("%s stays grouped by purpose", (name) => {
    const source = readFileSync(`${nativeStories}/${name}`, "utf8");
    expect(source, `${name} needs the same browsable grouping as the web workshop`).toMatch(
      /title:\s*"Loomarr (?:Foundations|Components)\//,
    );
  });
});
