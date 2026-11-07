import { surfGroups } from "@loomarr/fixtures";
import { describe, expect, it } from "vitest";

import { restoreTvSurfSelection } from "../index";

describe("TV Surf navigation", () => {
  it("restores the same channel in another group before falling back to the first occurrence", () => {
    const allOnly = surfGroups.filter((group) => group.kind === "all");
    expect(restoreTvSurfSelection(allOnly, { channelId: "ch-cartoons", group: "recent" })).toEqual({
      channelId: "ch-cartoons",
      group: "all",
    });
    expect(restoreTvSurfSelection(surfGroups, { channelId: "removed", group: "all" })).toEqual({
      channelId: "ch-cartoons",
      group: "recent",
    });
  });
});
