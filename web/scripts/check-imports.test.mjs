import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { findCatalogImports, findFrameworkImports } from "./check-imports.mjs";

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

describe("findFrameworkImports", () => {
  it("rejects direct Tamagui imports from product modules", () => {
    const source = `
      import { View } from "@tamagui/core";
      export { Button } from "tamagui";
    `;

    assert.deepEqual(
      findFrameworkImports(source).map(({ importPath }) => importPath),
      ["@tamagui/core", "tamagui"],
    );
  });

  it("accepts Loomarr-owned design-system imports", () => {
    assert.deepEqual(findFrameworkImports('import { Screen } from "@loomarr/design-system";'), []);
  });
});
