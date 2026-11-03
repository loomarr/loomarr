import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// "Which provider is this, and what does that mean?" has one answer: the provider registry the
// backend publishes (`GET /v1/filler/providers`). A comparison against "archive" or "youtube" is how
// a second answer starts: it works for the providers the author remembered, and a provider added
// later falls into the else branch. The Go side guards the same rule over its own source
// (internal/sourceprovider_architecture_test.go); this guards the web.
//
// It reads production source: tests, stories and fixtures are exempt because they stand in for the
// server's answers. Nothing else is allowlisted — a file that needs a provider's name asks the
// registry for it.
const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");
const PROVIDER_IDS = ["archive", "youtube"];
const LITERAL = new RegExp(`(["'\`])(${PROVIDER_IDS.join("|")})\\1`);

const exempt = (path: string) =>
  /\.(test|stories)\.tsx?$/.test(path) || /(^|\/)(test|generated|fixtures)\//.test(path);

const sources = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(entry) ? [path] : [];
  });

describe("provider identity", () => {
  it("is never a string literal in production source", () => {
    const offenders = sources(SRC)
      .map((path) => relative(SRC, path))
      .filter((path) => !exempt(path))
      .flatMap((path) =>
        readFileSync(join(SRC, path), "utf8")
          .split("\n")
          .flatMap((line, index) => (LITERAL.test(line) ? [`${path}:${index + 1}: ${line.trim()}`] : [])),
      );
    expect(offenders, "ask the provider registry (useFillerProviders) instead of naming a provider").toEqual(
      [],
    );
  });

  it("would catch a provider named in a comparison", () => {
    expect(LITERAL.test(`if (source.kind === "youtube") return;`)).toBe(true);
    expect(LITERAL.test("const id = `archive`;")).toBe(true);
    expect(LITERAL.test(`const label = "Archive.org";`)).toBe(false);
  });
});
