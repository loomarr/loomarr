import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as models from "../generated/model";
import * as barrel from "./index";
import * as mswBarrel from "./msw";
import * as zodBarrel from "./zod";

const here = dirname(fileURLToPath(import.meta.url));

const tagsIn = (dir: string) =>
  readdirSync(join(here, dir), { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => e.name);

const load = (path: string): Promise<Record<string, unknown>> => import(/* @vite-ignore */ join(here, path));

// The tags whose generated module exports something missing from (or different in) a flat barrel.
const missingFromFlat = async (barrelModule: object, tags: string[], pathOf: (tag: string) => string) => {
  const missing: string[] = [];
  for (const tag of tags) {
    const generated = await load(pathOf(tag));
    const absent = Object.entries(generated).some(
      ([name, value]) => (barrelModule as Record<string, unknown>)[name] !== value,
    );
    if (absent) missing.push(tag);
  }
  return missing;
};

// The barrel is hand-written (one `export * as <tag>Api` per generated tag) because the
// namespacing is deliberate — orval repeats its fetch-client helper enums in every tag
// file, so a flat re-export would collide.
//
// But hand-written means it silently falls behind: adding /v1/docs generated an entire
// `help` endpoint module that nothing exported, and the failure surfaced as "Module
// '@loomarr/api' has no exported member 'helpApi'" only when a page finally tried to use
// it. A generated module nobody can import is indistinguishable from an endpoint that
// was never built. Each check loads the generated modules and the barrel and compares what they
// export at runtime.
describe("@loomarr/api barrel", () => {
  // Orval emits enum-like DTO fields as runtime objects, so its model index is not type-only by
  // construction. A plain `export *` pulled every one of those files into Vite's startup graph:
  // opening the Guide waited for unrelated backup, database and filler DTO modules before auth
  // could even begin. Types disappear after compilation; the few enum objects used at runtime
  // are re-exported explicitly.
  it("does not re-export every generated DTO at runtime", () => {
    const runtimeModels = Object.keys(models);
    const onBarrel = runtimeModels.filter((name) => name in barrel);
    expect(runtimeModels.length).toBeGreaterThan(0);
    expect(onBarrel.length, "the barrel re-exports the generated model index at runtime").toBeLessThan(
      runtimeModels.length,
    );
  });

  it("exports a namespace for every generated endpoint tag", async () => {
    const namespaces: unknown[] = Object.values(barrel);
    const missing: string[] = [];
    for (const tag of tagsIn("../generated/endpoints")) {
      const generated = await load(`../generated/endpoints/${tag}/${tag}.ts`);
      if (!namespaces.includes(generated)) missing.push(tag);
    }
    expect(missing, `generated but not exported from the barrel: ${missing.join(", ")}`).toEqual([]);
  });

  // The zod barrel is the same hand-written-list-versus-generated-output problem, so it gets the
  // same guard rather than a convention nobody can enforce. ⚠ It is checked against the ZOD output
  // directory, not the endpoint one: they are deliberately not the same set — `events` is SSE and
  // has no request/response schemas to generate, so comparing against endpoints would fail forever
  // on a tag that is correct to be absent.
  it("exports every generated zod tag", async () => {
    const missing = await missingFromFlat(
      zodBarrel,
      tagsIn("../generated/zod"),
      (tag) => `../generated/zod/${tag}/${tag}.zod.ts`,
    );
    expect(missing, `generated but not exported from the zod barrel: ${missing.join(", ")}`).toEqual([]);
  });

  // ⚠ Checked against the ENDPOINT directory, not the zod one — the three sets are deliberately
  // different. MSW handlers exist for every endpoint tag including `events` (it has routes even
  // though it carries no request/response schemas), so the zod list is the odd one out and using
  // it here would let a whole tag's handlers go unexported without anything noticing.
  it("exports every generated msw tag", async () => {
    const missing = await missingFromFlat(
      mswBarrel,
      tagsIn("../generated/endpoints"),
      (tag) => `../generated/endpoints/${tag}/${tag}.msw.ts`,
    );
    expect(missing, `generated but not exported from the msw barrel: ${missing.join(", ")}`).toEqual([]);
  });
});
