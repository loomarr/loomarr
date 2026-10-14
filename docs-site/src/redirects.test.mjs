import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { PUBLISHED } from "./site-pages.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const docs = path.resolve(here, "../../docs");
const redirects = JSON.parse(readFileSync(path.join(here, "redirects.json"), "utf8"));

// Every route the site published before #1572 moved the tree (origin/main 381e9165). A reader's
// bookmark or a search result can carry any of them, so each must still be a page or redirect.
const PUBLISHED_BEFORE = [
  "install", "install/docker", "install/hardware", "install/upgrading", "install/monitoring",
  "help/quickstart", "help/troubleshooting", "help/filler", "help/integrations",
  "help/member-guide", "help/programming", "help/concepts",
  "dev", "dev/agents", "dev/ai", "dev/android-beta", "dev/ci", "dev/codegen", "dev/commands",
  "dev/dev-loop", "dev/graphify", "dev/playout-bench", "dev/releasing", "dev/setup",
  "dev/skills", "dev/testing",
  "configuration", "design", "programming-design", "config-design", "frontend-design",
  "integrations/media-server-livetv",
];

// A route is a page when docs/ has the file and the site publishes it.
const isPage = (route) => {
  const candidates = [`${route}.md`, `${route}/index.md`];
  return candidates.some(
    (rel) =>
      existsSync(path.join(docs, rel)) &&
      PUBLISHED.some((entry) => rel === entry || rel.startsWith(`${entry}/`)),
  );
};

test("every previously published route is still a page or redirects", () => {
  const lost = PUBLISHED_BEFORE.filter((r) => !isPage(r) && !redirects[`/${r}`]);
  assert.deepEqual(lost, []);
});

test("every redirect lands on a page", () => {
  const dangling = Object.entries(redirects).filter(([, to]) => !isPage(to.slice(1)));
  assert.deepEqual(dangling, []);
});
