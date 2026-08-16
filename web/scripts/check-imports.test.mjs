import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { findCatalogImports } from "./check-imports.mjs";

describe("findCatalogImports", () => {
  it("rejects value, type-only, re-export, and dynamic catalog dependencies", () => {
    const source = `
      import { Button } from "@/components/ui";
      import type { ChannelDTO } from "@loomarr/api";
      export * from "@/channels";
      const core = import("@loomarr/core");
    `;

    assert.deepEqual(
      findCatalogImports(source).map(({ importPath }) => importPath),
      ["@/components/ui", "@loomarr/api", "@/channels", "@loomarr/core"],
    );
  });

  it("accepts imports through specific public interfaces", () => {
    const source = `
      import { Button } from "@/components/ui/button";
      import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
      import * as channelsApi from "@loomarr/api/endpoints/channels";
      import type { EventHandlers } from "@loomarr/core/events";
    `;

    assert.deepEqual(findCatalogImports(source), []);
  });
});
