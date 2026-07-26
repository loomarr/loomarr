import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const src = (rel: string) => readFileSync(join(here, "..", rel), "utf8");

// config-design §5 names FOUR inline-commit exceptions, and they are all verbs: select a model,
// pull a model, regenerate a secret, run a job now. Each commits immediately and deliberately
// sits OUTSIDE the cross-tab save bar, because you do not stage an action — a Save button next
// to "Run now" is nonsense, and staging "regenerate a secret" would be worse than nonsense
// since the operation is destructive the moment it runs.
//
// V9 lifted the edit buffer into the layout, which made it newly easy to sweep these in "for
// consistency". This asserts they stay out. It is a source-level check because the property is
// architectural — "this surface does not participate in the buffer" — and a rendering test
// would pass just as happily with the action wired to a staged edit that nobody saved.
describe("the four inline-commit exceptions stay out of the save bar", () => {
  const cases = [
    ["select/pull a model", "settings/ai-model-settings/ai-model-settings.tsx"],
    ["regenerate a secret", "settings/secrets-settings/secrets-settings.tsx"],
    ["run a job now", "settings/tasks-page/tasks-page.tsx"],
  ] as const;

  for (const [what, path] of cases) {
    it(`${what} does not read the shared edit buffer`, () => {
      expect(src(path)).not.toContain("useSettingsEdits");
    });

    // …and still commits: each holds a mutation it fires directly. Without this the first
    // assertion would pass for a surface that had simply stopped doing anything.
    it(`${what} commits immediately`, () => {
      expect(src(path)).toMatch(/\.mutate\(/);
    });
  }

  // The save bar's own host DOES use the buffer — the control case, so the assertions above
  // are testing a real distinction rather than a string that appears nowhere.
  it("the settings page itself uses the shared buffer", () => {
    expect(src("settings/settings-page/settings-page.tsx")).toContain("useSettingsEdits");
  });
});
