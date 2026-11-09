import { existsSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as loomarr from "@/components/loomarr";

// LIBRARY HEALTH (frontend-design §5.1, §5.1a, §5.1b).
//
// Story coverage was the original rule: a component without a story is invisible to the visual
// and a11y gates. This file now enforces the rest of what makes a 46-component library usable,
// because every one of those properties was a convention that decayed exactly as fast as the
// library grew:
//
//   coverage      — every Layer 2 component (the primitives are the design system's, storied there)
//   grouping      — every story title carries a domain prefix, so the sidebar cannot flatten
//   design pages  — the palette/type/spacing/token pages exist and are snapshotted
//
// None of this was catchable by review. A flat namespace and nine unstoried primitives are
// invisible in a diff; they only surface when someone goes looking.
const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");
const LOOMARR_DIR = join(SRC, "components", "loomarr");
const DESIGN_DIR = join(SRC, "design");

const toKebab = (name: string): string => name.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();

// Runtime component exports from a barrel = capitalized functions (types are erased).
const componentsOf = (mod: object): string[] =>
  Object.entries(mod)
    .filter(([name, value]) => /^[A-Z]/.test(name) && typeof value === "function")
    .map(([name]) => name);

// Layer 2 is grouped by domain, so a component's story lives at <domain>/<kebab>/<kebab>.stories.tsx
// for exactly one domain. Searching rather than hardcoding the map means moving a component
// between domains does not require touching this test.
const domainDirs = readdirSync(LOOMARR_DIR, { withFileTypes: true })
  .filter((e) => e.isDirectory())
  .map((e) => e.name);

const findStory = (kebab: string): string | null => {
  for (const domain of domainDirs) {
    const p = join(LOOMARR_DIR, domain, kebab, `${kebab}.stories.tsx`);
    if (existsSync(p)) return p;
  }
  return null;
};

const loomarrComponents = componentsOf(loomarr);

describe("Storybook coverage", () => {
  it("sees the full Layer-2 component vocabulary", () => {
    expect(loomarrComponents.length).toBeGreaterThanOrEqual(40);
  });

  it.each(loomarrComponents)("%s has a co-located *.stories.tsx", (name) => {
    const kebab = toKebab(name);
    expect(findStory(kebab), `${name}: expected <domain>/${kebab}/${kebab}.stories.tsx`).not.toBeNull();
  });
});

// §5.1a: the sidebar has to stay navigable. A bare `Loomarr/<Name>` title is what 46 components
// under one flat namespace looked like — every story present, and none of them findable.
describe("Story grouping", () => {
  // The title Storybook files a story under is its module's CSF meta — the default export — so
  // the check reads it from the loaded module, not from the file's text.
  const storyModules = import.meta.glob<{ default: { title?: string } }>([
    "../components/loomarr/**/*.stories.tsx",
    "../design/**/*.stories.tsx",
  ]);
  const storyFiles = Object.keys(storyModules).sort();

  it("has stories to check", () => {
    expect(storyFiles.length).toBeGreaterThanOrEqual(55);
  });

  it.each(storyFiles.map((f) => [f.replace(/^\.\.\//, "src/"), f] as const))(
    "%s carries a group prefix",
    async (_label, file) => {
      const title = (await storyModules[file]?.())?.default.title;
      expect(title, `${file}: no title in meta`).toBeDefined();
      // "Channels/ChannelCard" — a group, then the component.
      expect(title, `${title}: needs a <Group>/<Name> title`).toMatch(/^[A-Za-z]+\/.+/);
      expect(title?.startsWith("Loomarr/"), `${title}: "Loomarr/" is the flat namespace §5.1a replaced`).toBe(
        false,
      );
    },
  );
});

// §5.1a: the design system documents itself. These pages read the GENERATED tokens, so they
// double as a live check that the artifacts are importable and shaped as expected.
describe("Design pages", () => {
  it.each(["palette", "typography", "spacing", "tokens"])("Design/%s exists", (page) => {
    expect(existsSync(join(DESIGN_DIR, page, `${page}.stories.tsx`)), `missing design/${page}`).toBe(true);
  });
});
