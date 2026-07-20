import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));

// The barrel is hand-written (one `export * as <tag>Api` per generated tag) because the
// namespacing is deliberate — orval repeats its fetch-client helper enums in every tag
// file, so a flat re-export would collide.
//
// But hand-written means it silently falls behind: adding /v1/docs generated an entire
// `help` endpoint module that nothing exported, and the failure surfaced as "Module
// '@loomarr/api' has no exported member 'helpApi'" only when a page finally tried to use
// it. A generated module nobody can import is indistinguishable from an endpoint that
// was never built.
describe("@loomarr/api barrel", () => {
  it("exports a namespace for every generated endpoint tag", () => {
    const generated = readdirSync(join(here, "../generated/endpoints"), { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => e.name);
    const barrel = readFileSync(join(here, "index.ts"), "utf8");

    const missing = generated.filter(
      (tag) => !barrel.includes(`from "../generated/endpoints/${tag}/${tag}"`),
    );
    expect(missing, `generated but not exported from the barrel: ${missing.join(", ")}`).toEqual([]);
  });
});
