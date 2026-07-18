import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as loomarr from "@/components/loomarr";

// Story coverage (frontend-design §5.1): every Layer-2 component MUST have a co-located
// `*.stories.tsx`. A component that ships without a story fails this test — the
// successor to "unregistered components are a lint error". It keeps the visual + a11y
// gate from silently skipping a component.
const LOOMARR_DIR = join(dirname(fileURLToPath(import.meta.url)), "..", "components", "loomarr");

const toKebab = (name: string): string => name.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();

// Runtime component exports from the barrel = capitalized functions (types are erased).
const componentNames = Object.entries(loomarr)
  .filter(([name, value]) => /^[A-Z]/.test(name) && typeof value === "function")
  .map(([name]) => name);

describe("Storybook coverage", () => {
  it("sees the full Layer-2 component vocabulary", () => {
    expect(componentNames.length).toBeGreaterThanOrEqual(14);
  });

  it.each(componentNames)("%s has a co-located *.stories.tsx", (name) => {
    const kebab = toKebab(name);
    const storyPath = join(LOOMARR_DIR, kebab, `${kebab}.stories.tsx`);
    expect(existsSync(storyPath), `${name}: expected ${kebab}/${kebab}.stories.tsx`).toBe(true);
  });
});
