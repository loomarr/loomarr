import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { sourceTextReads, webTestFiles } from "./source-text-tests.mjs";

const WEB = fileURLToPath(new URL("..", import.meta.url));

// sourceTextAllowlist maps a web-relative test file to why it still reads source as text. It is a
// ratchet: an entry that no longer reads source fails, and a new entry needs a reason.
//
// TEMPORARY entries are owed a conversion. The native app shells (apps/*/app, apps/tv/src) and the
// on-device workshop (.rnstorybook, native-stories) resolve their imports only through an app's
// Metro bundle, so no JS test runner here can load them yet; each converts once its logic moves
// into a vitest-covered package or the workshop gets a resolvable test home.
const sourceTextAllowlist = {
  "apps/tv/tests/config.test.mjs":
    "TEMPORARY: matches the TV app root's source (src/app.tsx) and paired-client.ts; move shell wiring into @loomarr/player and test it there",
  "packages/design-system/tests/native-story-theme.test.ts":
    "TEMPORARY: matches .rnstorybook/preview.tsx and native-stories CSF; load them as modules once the workshop resolves outside Metro",
  "scripts/capture-tv-native-references.test.mjs":
    "TEMPORARY: matches .rnstorybook/tv-ui.tsx; render the capture controller once the workshop resolves outside Metro",
  "apps/web/tests/e2e/temporal-reviewer.spec.ts":
    "executes the reviewer HTML embedded in frozen research Go source in a browser; it reads the template to run it, not to match it",
};

describe("sourceTextReads", () => {
  const trips = {
    "literal path": `import { readFileSync } from "node:fs";
      expect(readFileSync("src/main.tsx", "utf8")).toContain("<Provider");`,
    "new URL": `const shell = await readFile(new URL("../app/index.tsx", import.meta.url), "utf8");`,
    "helper parameter": `const src = (rel) => readFileSync(join(here, "..", rel), "utf8");
      expect(src("settings/page/page.tsx")).toContain("useEdits");`,
    // biome-ignore lint/suspicious/noTemplateCurlyInString: the fixture is source text holding a template literal
    "template path": "for (const name of names) readFileSync(`${dir}/${name}.stories.tsx`, 'utf8');",
    "it.each table": `it.each(["a.ts", "b.ts"])("%s", (file) => { readFileSync(file, "utf8"); });`,
    "walk over source": `const files = readdirSync(dir).filter((n) => n.endsWith(".stories.tsx"));
      for (const f of files) readFileSync(join(dir, f), "utf8");`,
    "raw import": `import source from "./worker.ts?raw";`,
    "raw glob": `const all = import.meta.glob("./**/*.tsx", { query: "?raw", eager: true });`,
  };
  const passes = {
    "non-source artefact": `const md = readFileSync(join(root, "docs/guides/troubleshooting.md"), "utf8");`,
    "json config": `const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));`,
    "temp output": `const dir = mkdtempSync(join(tmpdir(), "x-")); readFileSync(join(dir, "report.js"), "utf8");`,
    "ast input": `import { parse } from "@babel/parser";
      const text = readFileSync("src/a.tsx", "utf8");
      const program = parse(text, { plugins: ["typescript", "jsx"] });`,
    "lists names only": `const stories = readdirSync(dir).filter((n) => n.endsWith(".stories.tsx"));
      expect(stories.length).toBeGreaterThan(0);`,
  };
  for (const [name, source] of Object.entries(trips)) {
    it(`flags ${name}`, () => {
      assert.notDeepEqual(sourceTextReads(source, "fixture.test.ts"), [], name);
    });
  }
  for (const [name, source] of Object.entries(passes)) {
    it(`passes ${name}`, () => {
      assert.deepEqual(sourceTextReads(source, "fixture.test.ts"), [], name);
    });
  }
});

describe("web tests assert on behaviour, not on source text", () => {
  const files = webTestFiles(WEB);
  const violating = new Map();
  for (const file of files) {
    const reads = sourceTextReads(readFileSync(join(WEB, file), "utf8"), file);
    if (reads.length > 0) violating.set(file, reads);
  }

  it("examines the web test tree", () => {
    assert.ok(files.length > 50, `found only ${files.length} test files; the walk is missing the tree`);
  });

  it("no test reads source as text outside the allowlist", () => {
    const offenders = [...violating]
      .filter(([file]) => !(file in sourceTextAllowlist))
      .flatMap(([file, reads]) => reads.map((r) => `${file}:${r.line} ${r.reason}`));
    assert.deepEqual(
      offenders,
      [],
      "Tests assert on behaviour, not on source text: render or call the code, or parse it (AGENTS.md, Tests).",
    );
  });

  it("every allowlist entry still reads source and says why", () => {
    for (const [file, reason] of Object.entries(sourceTextAllowlist)) {
      assert.ok(reason.trim() !== "", `${file} has no reason`);
      assert.ok(violating.has(file), `${file} no longer reads source as text; remove it from the allowlist`);
    }
  });
});
