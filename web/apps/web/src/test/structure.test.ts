import { readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Folder-per-module conformance (FE conventions rule 3 + 5). This test exists because
// the convention was written down, agreed, and then broken twice anyway — `src/auth/`
// and `src/wizard/` both shipped flat before anyone noticed. A rule a human has to
// remember is a rule that erodes; this is the same move the story-coverage test makes
// for "every component has a story", and the GritQL plugins make for arrow functions.
//
// The rule: inside a module container, every unit is its OWN FOLDER holding
// `name.ts(x)`, optional `name.type.ts`, its test, and an `index.ts` barrel. No loose
// implementation files sitting directly in the container.
const SRC = join(dirname(fileURLToPath(import.meta.url)), "..");

// Directories whose children are modules. `src/test` is deliberately absent: it holds
// cross-cutting specs and vitest setup, which belong to no single module.
const CONTAINERS = ["auth", "wizard", "lib", "components/ui", "components/loomarr", "routes"];

const isDir = (p: string) => statSync(p).isDirectory();
const entries = (p: string) => readdirSync(p);

// A route tree is file-based (TanStack owns those filenames), so `routes/` is checked
// only for the absence of stray non-route modules — its files ARE the convention.
const ROUTE_FILE = /^(__root|_authed|[a-z0-9$._-]+)\.tsx?$/;

describe("folder-per-module conformance", () => {
  for (const container of CONTAINERS.filter((c) => c !== "routes")) {
    it(`${container}/ has no loose implementation files`, () => {
      const dir = join(SRC, container);
      const loose = entries(dir).filter(
        (e) => !isDir(join(dir, e)) && e !== "index.ts" && !e.endsWith(".css"),
      );
      expect(loose, `move each into its own folder with an index.ts barrel`).toEqual([]);
    });

    it(`${container}/ every module folder has an index.ts barrel`, () => {
      const dir = join(SRC, container);
      const missing = entries(dir)
        .filter((e) => isDir(join(dir, e)))
        .filter((e) => !entries(join(dir, e)).includes("index.ts"));
      expect(missing, "a module folder must export through index.ts (rule 5)").toEqual([]);
    });

    it(`${container}/ types live in *.type.ts, never in an implementation file`, () => {
      const dir = join(SRC, container);
      for (const mod of entries(dir).filter((e) => isDir(join(dir, e)))) {
        const files = entries(join(dir, mod));
        const impl = files.filter(
          (f) => /\.tsx?$/.test(f) && !f.endsWith(".type.ts") && !/\.(test|stories)\./.test(f),
        );
        // Every implementation file must be named for its folder — that's what makes
        // `name.type.ts` / `name.test.tsx` co-location unambiguous.
        for (const f of impl) {
          if (f === "index.ts") continue;
          expect(f.replace(/\.tsx?$/, ""), `${container}/${mod}: file should be named for its folder`).toBe(
            mod,
          );
        }
      }
    });
  }

  it("routes/ contains only route modules (file-based routing owns these names)", () => {
    const dir = join(SRC, "routes");
    const stray = entries(dir).filter((e) => !isDir(join(dir, e)) && !ROUTE_FILE.test(e));
    expect(stray, "non-route helpers belong in a feature folder, not routes/").toEqual([]);
  });
});
