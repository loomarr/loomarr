import { describe, expect, it } from "vitest";

import { resolveFontWeight } from "../src/primitives/primitives";

describe("universal typography weights", () => {
  it("preserves variable-font weights on web", () => {
    expect(resolveFontWeight("550", true)).toBe("550");
    expect(resolveFontWeight("650", true)).toBe("650");
  });

  it("uses supported system-font weights on native clients", () => {
    expect(resolveFontWeight("550", false)).toBe("600");
    expect(resolveFontWeight("650", false)).toBe("700");
  });
});
