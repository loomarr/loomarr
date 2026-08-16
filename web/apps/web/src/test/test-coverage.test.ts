import { existsSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// COLOCATED TEST COVERAGE (FE conventions rule 6: "each component/module ships a Vitest +
// Testing Library test covering its meaningful states — not a token smoke test").
//
// The rule was written down and then decayed for the ordinary reason: nothing checked it. By
// the time anyone counted, 45 modules had none — including four `components/ui` primitives and
// every wizard step, which is to say the onboarding path a first-run operator walks.
//
// ⚠ **This is deliberately NOT the story-coverage gate's shape.** That one enumerates the
// component BARRELS, because frontend-design §5.1 scopes stories to Layer 1 and Layer 2 and
// says pages merely compose them. Tests have no such scope: a page holding eight useState calls
// and a fan-out of queries is exactly the thing a unit test is for. So this walks the source
// tree instead.
//
// ⚠ **`routes/` is exempt, and the reason is coverage rather than convenience.** A route file's
// contract is its `validateSearch`, its `beforeLoad` guard and its loader — none of which mean
// anything without a router around them, and all of which ARE exercised by the router-level
// specs in this directory (app-router, wizard-router, dashboard-lockout). A colocated unit test
// would have to stand a router up to say anything at all, at which point it is one of those
// files. What this exemption does NOT cover is a page BODY that grew inside a route file; that
// belongs in `<feature>/<name>-page/` with a test beside it, which is a move, not a new test.
//
// ⚠ **The allowlist below is a debt register, not an exemption list.** Every entry is a module
// that predates this gate. New code cannot join it — adding a name here to make a red build
// green is the exact move this file exists to prevent, and a reviewer seeing this list grow
// should reject the PR. Entries come OFF as tests land; when it is empty, delete it and the
// branch that reads it.
const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");

// Not module implementations: barrels, prop types, ambient declarations, tests, stories.
const isImplementation = (file: string) =>
  /\.tsx?$/.test(file) &&
  file !== "index.ts" &&
  !file.endsWith(".type.ts") &&
  !file.endsWith(".d.ts") &&
  !/\.(test|stories)\./.test(file);

// Directories holding no unit of behavior a COLOCATED test could reach.
//  - `test`     these cross-cutting specs themselves
//  - `design`   Storybook-only token gallery pages; their contract is the visual baseline, and
//               a jsdom assertion on a swatch would test that a hex string equals itself
//  - `routes`   covered by the router-level specs — see the warning above
const EXEMPT_DIRS = new Set(["test", "design", "routes", "node_modules", "dist"]);
const EXEMPT_FILES = new Set(["main.tsx", "routeTree.gen.ts"]);

// Modules that predate this gate. See the warning above: this list only shrinks.
const KNOWN_GAPS = new Set([
  "auth/me-query/me-query.ts",
  "channels/guide-page/guide-page.tsx",
  "channels/guide-window/guide-window.ts",
  "channels/use-delete-confirm/use-delete-confirm.ts",
  "channels/use-tunarr-ready/use-tunarr-ready.ts",
  "components/loomarr/channels/channel-rules-editor/label/label.ts",
  "components/loomarr/channels/channel-rules-editor/presets/presets.ts",
  "components/loomarr/settings/settings-save-bar/settings-save-bar.tsx",
  "components/ui/checkbox/checkbox.tsx",
  "dashboard/restart-watch-provider/restart-watch-provider.tsx",
  "dashboard/use-restart-watch/use-restart-watch.ts",
  // filler-criteria.tsx left this register in V51f — it grew tests when the era field gained
  // three states worth defending.
  // filler-clip-list.tsx left it when the dead-pin and pod_max fixes landed (#237/#238).
  // ⚠ Both removals in one place: V51f and this branch each retired a DIFFERENT entry, which
  // conflicts here by construction. The resolution keeps neither path — a merge that kept one
  // would re-exempt a module that now has tests, and the third assertion below is what catches
  // that rather than leaving the register quietly wrong.
  "filler/channel-filler/use-filler-catalog/use-filler-catalog.ts",
  "filler/clip-tag-dialog/clip-tag-dialog.tsx",
  "help/help-page/help-page.tsx",
  "help/search-docs/search-docs.ts",
  "lib/use-document-title/use-document-title.ts",
  "palette/command-palette/command-palette.tsx",
  "palette/use-command-shortcut/use-command-shortcut.ts",
  "palette/use-palette-results/use-palette-results.ts",
  "people/create-local-panel/create-local-panel.tsx",
  "people/import-panel/import-panel.tsx",
  "people/users-page/users-page.tsx",
  "queue/approval-history/approval-history.tsx",
  "settings/ai-model-settings/ai-model-settings.tsx",
  "settings/secrets-settings/secrets-settings.tsx",
  "settings/settings-page/settings-page.tsx",
  "settings/use-settings-entries/use-settings-entries.ts",
  "suggest/use-suggestion-run/use-suggestion-run.ts",
  "wizard/bootstrap-step/bootstrap-step.tsx",
  "wizard/checklist-step/checklist-step.tsx",
  "wizard/playout-step/playout-step.tsx",
  "wizard/tunarr-library-step/tunarr-library-step.tsx",
  "wizard/use-complete-setup/use-complete-setup.ts",
  "wizard/setup-completed/setup-completed.ts",
  "components/loomarr/channels/channel-icon-field/channel-icon-field.tsx",
  "lib/use-copied/use-copied.ts",
  "settings/settings-edits/settings-edits.tsx",
  "suggest/round/round.ts",
]);

const modulesWithoutTests = (): string[] => {
  const found: string[] = [];

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const p = join(dir, entry);
      if (statSync(p).isDirectory()) {
        if (!EXEMPT_DIRS.has(entry)) walk(p);
        continue;
      }
      if (!isImplementation(entry) || EXEMPT_FILES.has(entry)) continue;

      const base = p.replace(/\.tsx?$/, "");
      if (existsSync(`${base}.test.ts`) || existsSync(`${base}.test.tsx`)) continue;
      found.push(relative(SRC, p));
    }
  };

  walk(SRC);
  return found.sort();
};

describe("colocated test coverage", () => {
  const missing = modulesWithoutTests();

  it("has modules to check", () => {
    // Guards against the walk silently matching nothing — a gate that checks zero files passes
    // forever and says nothing, which is worse than not having it.
    expect(missing.length + KNOWN_GAPS.size).toBeGreaterThan(50);
  });

  it("every module outside the debt register ships a test", () => {
    const undeclared = missing.filter((m) => !KNOWN_GAPS.has(m));
    expect(
      undeclared,
      `these modules have no colocated *.test.ts(x). Write one — do NOT add them to KNOWN_GAPS:\n  ${undeclared.join("\n  ")}`,
    ).toEqual([]);
  });

  // Without this, the register rots: a module gets its test, the entry stays, and the list stops
  // describing anything real. It also catches a rename, where the stale path would otherwise
  // silently exempt nothing at all.
  it("the debt register lists only modules that are still missing tests", () => {
    const stale = [...KNOWN_GAPS].filter((g) => !missing.includes(g)).sort();
    expect(
      stale,
      `these are no longer missing tests (or were renamed) — remove them from KNOWN_GAPS:\n  ${stale.join("\n  ")}`,
    ).toEqual([]);
  });
});
