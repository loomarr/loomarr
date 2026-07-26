import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as loomarr from "@/components/loomarr";
import * as ui from "@/components/ui";

// LIBRARY HEALTH (frontend-design §5.1, §5.1a, §5.1b).
//
// Story coverage was the original rule: a component without a story is invisible to the visual
// and a11y gates. This file now enforces the rest of what makes a 46-component library usable,
// because every one of those properties was a convention that decayed exactly as fast as the
// library grew:
//
//   coverage      — Layer 1 AND Layer 2 (primitives were exempt; the token layer restyles them,
//                   so they are the LAST place to skip pixel coverage)
//   grouping      — every story title carries a domain prefix, so the sidebar cannot flatten
//   design pages  — the palette/type/spacing/token pages exist and are snapshotted
//
// None of this was catchable by review. A flat namespace and nine unstoried primitives are
// invisible in a diff; they only surface when someone goes looking.
const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");
const LOOMARR_DIR = join(SRC, "components", "loomarr");
const UI_DIR = join(SRC, "components", "ui");
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

const allStoryFiles = (dir: string): string[] => {
  const out: string[] = [];
  const walk = (d: string) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      const p = join(d, e.name);
      if (e.isDirectory()) walk(p);
      else if (e.name.endsWith(".stories.tsx")) out.push(p);
    }
  };
  walk(dir);
  return out;
};

const loomarrComponents = componentsOf(loomarr);
const uiComponents = componentsOf(ui);

describe("Storybook coverage", () => {
  it("sees the full Layer-2 component vocabulary", () => {
    expect(loomarrComponents.length).toBeGreaterThanOrEqual(40);
  });

  it.each(loomarrComponents)("%s has a co-located *.stories.tsx", (name) => {
    const kebab = toKebab(name);
    expect(findStory(kebab), `${name}: expected <domain>/${kebab}/${kebab}.stories.tsx`).not.toBeNull();
  });
});

// §5.1: primitives are in scope. They are restyled through the token layer, so a palette edit
// moves all of them at once — with no stories, neither the gallery nor the visual suite would
// show it. `Button`'s dark-on-accent text and `Badge`'s -300 stops are contrast calibrations
// that were learned the hard way; these stories are what keeps them from silently regressing.
describe("Primitive coverage", () => {
  // Radix re-exports (SelectTrigger, DialogHeader…) are parts of a primitive, not primitives —
  // the story lives with the folder, so coverage is checked per FOLDER rather than per export.
  const primitiveFolders = readdirSync(UI_DIR, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => e.name);

  it("sees the primitive layer", () => {
    expect(primitiveFolders.length).toBeGreaterThanOrEqual(9);
    expect(uiComponents.length).toBeGreaterThan(0);
  });

  it.each(primitiveFolders)("%s has a co-located *.stories.tsx", (folder) => {
    const p = join(UI_DIR, folder, `${folder}.stories.tsx`);
    expect(existsSync(p), `ui/${folder}: expected ${folder}/${folder}.stories.tsx`).toBe(true);
  });
});

// §5.1a: the sidebar has to stay navigable. A bare `Loomarr/<Name>` title is what 46 components
// under one flat namespace looked like — every story present, and none of them findable.
describe("Story grouping", () => {
  // Anchored to the META block, not to the first `title:` in the file. Several stories carry
  // fixture data with its own `title` ("The Next Generation" — an episode name), and matching
  // the first occurrence reported those as malformed story titles.
  const titleOf = (file: string): string | null => {
    const src = readFileSync(file, "utf8");
    const meta = /const meta\s*=\s*\{[\s\S]*?\n\}/.exec(src)?.[0] ?? src;
    return /title:\s*"([^"]+)"/.exec(meta)?.[1] ?? null;
  };

  const storyFiles = [...allStoryFiles(LOOMARR_DIR), ...allStoryFiles(UI_DIR), ...allStoryFiles(DESIGN_DIR)];

  it("has stories to check", () => {
    expect(storyFiles.length).toBeGreaterThanOrEqual(55);
  });

  it.each(storyFiles.map((f) => [f.replace(SRC, "src"), f] as const))(
    "%s carries a group prefix",
    (_label, file) => {
      const title = titleOf(file);
      expect(title, `${file}: no title in meta`).not.toBeNull();
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
