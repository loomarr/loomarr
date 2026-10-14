// Contract for playwright.quarantine.ts, run by the tuner job before its matrix:
//   node --experimental-strip-types --test playwright.quarantine.test.ts
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { applyFlakeQuarantine } from "./playwright.quarantine.ts";

const spec = "tuner-surf.spec.ts";
const title = "100-channel tuner meets surf latency and latest-request-wins gates";
const projects = [{ name: "chromium" }, { name: "firefox" }, { name: "webkit" }];

function quarantineFile(lines: string[]): string {
  const file = join(mkdtempSync(join(tmpdir(), "quarantine-")), "active.tsv");
  writeFileSync(file, lines.map((line) => `${line}\n`).join(""));
  return file;
}

// Whether a project runs a test, from the string Playwright matches grep against (measured with
// `playwright test --list --grep`): "<project> <spec> <title>".
function runs(
  project: { name: string; grep?: unknown; grepInvert?: unknown },
  name = title,
  file = spec,
): boolean {
  const subject = `${project.name} ${file} ${name}`;
  const grep = project.grep as RegExp | undefined;
  const grepInvert = project.grepInvert as RegExp | undefined;
  return (grep === undefined || grep.test(subject)) && !(grepInvert?.test(subject) ?? false);
}

test("off, or unset, runs every project unchanged", () => {
  assert.equal(applyFlakeQuarantine(projects, {}), projects);
  assert.equal(applyFlakeQuarantine(projects, { PLAYWRIGHT_QUARANTINE: "off" }), projects);
});

test("exclude keeps every project and drops only the quarantined pair", () => {
  const file = quarantineFile([`webkit\t${spec}\t${title}\t1443`]);
  const got = applyFlakeQuarantine(projects, {
    PLAYWRIGHT_QUARANTINE: "exclude",
    PLAYWRIGHT_QUARANTINE_FILE: file,
  });
  assert.deepEqual(
    got.map((p) => p.name),
    ["chromium", "firefox", "webkit"],
  );
  const [chromium, firefox, webkit] = got;
  assert.ok(runs(chromium) && runs(firefox));
  assert.ok(!runs(webkit));
  // Every other test in the quarantined project stays required, including near misses.
  assert.ok(runs(webkit, "an unrelated test"));
  assert.ok(runs(webkit, `slow ${title}`));
  assert.ok(runs(webkit, `${title} again`));
  assert.ok(runs(webkit, title, "other.spec.ts"));
});

test("only runs just the quarantined pairs", () => {
  const file = quarantineFile([`webkit\t${spec}\t${title}\t1443`, `firefox\t${spec}\t${title}\t1492`]);
  const got = applyFlakeQuarantine(projects, {
    PLAYWRIGHT_QUARANTINE: "only",
    PLAYWRIGHT_QUARANTINE_FILE: file,
  });
  assert.deepEqual(
    got.map((p) => p.name),
    ["firefox", "webkit"],
  );
  for (const project of got) {
    assert.ok(runs(project));
    assert.ok(!runs(project, "an unrelated test"));
  }
});

test("titles are literal text, not patterns", () => {
  const file = quarantineFile([`webkit\t${spec}\ta (b) c.d\t7`]);
  const [, , webkit] = applyFlakeQuarantine(projects, {
    PLAYWRIGHT_QUARANTINE: "exclude",
    PLAYWRIGHT_QUARANTINE_FILE: file,
  });
  assert.ok(!runs(webkit, "a (b) c.d"));
  assert.ok(runs(webkit, "a b cxd"));
});

test("anything unexpected fails closed", () => {
  const good = quarantineFile([`webkit\t${spec}\t${title}\t1443`]);
  const cases: Record<string, NodeJS.ProcessEnv> = {
    "unknown mode": { PLAYWRIGHT_QUARANTINE: "exlude", PLAYWRIGHT_QUARANTINE_FILE: good },
    "missing file variable": { PLAYWRIGHT_QUARANTINE: "exclude" },
    "unknown project": {
      PLAYWRIGHT_QUARANTINE: "exclude",
      PLAYWRIGHT_QUARANTINE_FILE: quarantineFile([`safari\t${spec}\t${title}\t1443`]),
    },
    "missing issue": {
      PLAYWRIGHT_QUARANTINE: "exclude",
      PLAYWRIGHT_QUARANTINE_FILE: quarantineFile([`webkit\t${spec}\t${title}`]),
    },
    "extra field": {
      PLAYWRIGHT_QUARANTINE: "exclude",
      PLAYWRIGHT_QUARANTINE_FILE: quarantineFile([`webkit\t${spec}\t${title}\t1443\tx`]),
    },
    "nothing to run in only": {
      PLAYWRIGHT_QUARANTINE: "only",
      PLAYWRIGHT_QUARANTINE_FILE: quarantineFile([]),
    },
  };
  for (const [name, env] of Object.entries(cases)) {
    assert.throws(() => applyFlakeQuarantine(projects, env), Error, name);
  }
});
