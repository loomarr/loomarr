import { describe, expect, it } from "vitest";

import { NavTabs } from "../index";

// NavTabs has no native adapter; its `.tsx` twin refuses, naming the component to use instead.
describe("NavTabs (native)", () => {
  it("throws, pointing at TabBar", () => {
    expect(() => NavTabs({ label: "Sections", tabs: [] } as never)).toThrow(/TabBar/);
  });
});
