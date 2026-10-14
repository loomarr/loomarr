import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";

const workspace = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));

describe("shared client resource bounds", () => {
  it("bundles mobile and TV clients one at a time", () => {
    assert.match(workspace.scripts["clients:check"], /turbo run bundle --concurrency=1/);
  });
});
