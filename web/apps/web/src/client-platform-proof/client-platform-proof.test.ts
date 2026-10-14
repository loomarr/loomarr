import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "vitest";

describe("client platform proof entry", () => {
  it("is the executable module loaded by the isolated proof page", async () => {
    const html = await readFile("client-platform-proof.html", "utf8");

    assert.match(html, /src="\/src\/client-platform-proof\/client-platform-proof\.tsx"/);
  });
});
