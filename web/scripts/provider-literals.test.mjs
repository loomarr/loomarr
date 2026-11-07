import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

// "Which provider is this, and what does that mean?" has one answer: the provider registry the
// backend publishes (`GET /v1/filler/providers`). A comparison against "archive" or "youtube" is how
// a second answer starts: it works for the providers the author remembered, and a provider added
// later falls into the else branch. The Go side guards the same rule over its own source
// (internal/sourceprovider_architecture_test.go); this guards the web app.
//
// It parses production source and looks at its string literals: tests, stories and fixtures are
// exempt because they stand in for the server's answers. Nothing else is allowlisted — a file that
// needs a provider's name asks the registry for it (useFillerProviders).
const SRC = fileURLToPath(new URL("../apps/web/src/", import.meta.url));
const PROVIDER_IDS = new Set(["archive", "youtube"]);

const exempt = (path) =>
  /\.(test|stories)\.tsx?$/.test(path) || /(^|\/)(test|generated|fixtures)\//.test(path);

const productionSources = (dir) =>
  readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return productionSources(path);
    const rel = relative(SRC, path).split(sep).join("/");
    return /\.tsx?$/.test(entry.name) && !exempt(rel) ? [rel] : [];
  });

// providerLiterals returns the line of every string or plain template literal in the program that
// is exactly a provider id.
const providerLiterals = (program) => {
  const lines = [];
  const visit = (node) => {
    if (!node || typeof node.type !== "string") return;
    if (node.type === "StringLiteral" && PROVIDER_IDS.has(node.value)) lines.push(node.loc.start.line);
    if (
      node.type === "TemplateLiteral" &&
      node.expressions.length === 0 &&
      PROVIDER_IDS.has(node.quasis[0].value.cooked)
    ) {
      lines.push(node.loc.start.line);
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === "loc" || key === "leadingComments" || key === "trailingComments") continue;
      for (const child of Array.isArray(value) ? value : [value]) {
        if (child && typeof child === "object") visit(child);
      }
    }
  };
  visit(program);
  return lines;
};

// JSX only for .tsx: in a .ts file `<T>value` is a type assertion, which the JSX plugin misreads.
const parseSource = (code, path = "snippet.ts") =>
  parse(code, {
    sourceType: "module",
    plugins: path.endsWith(".tsx") ? ["typescript", "jsx"] : ["typescript"],
  }).program;

describe("provider identity", () => {
  it("is never a string literal in production source", () => {
    const files = productionSources(SRC);
    assert.ok(
      files.length > 100,
      `only ${files.length} production sources found; the walk stopped finding them`,
    );
    const offenders = files.flatMap((path) => {
      const code = readFileSync(join(SRC, path), "utf8");
      return providerLiterals(parseSource(code, path)).map((line) => `${path}:${line}`);
    });
    assert.deepEqual(
      offenders,
      [],
      "ask the provider registry (useFillerProviders) instead of naming a provider",
    );
  });

  it("would catch a provider named in a comparison", () => {
    assert.deepEqual(providerLiterals(parseSource(`const isTube = source.kind === "youtube";`)), [1]);
    assert.deepEqual(providerLiterals(parseSource("const id = `archive`;")), [1]);
    assert.deepEqual(providerLiterals(parseSource(`const label = "Archive.org";`)), []);
    assert.deepEqual(providerLiterals(parseSource("// a youtube comment\nconst x = 1;")), []);
  });
});
