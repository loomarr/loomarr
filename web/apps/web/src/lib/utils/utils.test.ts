import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("joins truthy class values", () => {
    expect(cn("a", false, "b", undefined, "c")).toBe("a b c");
  });

  it("resolves conflicting tailwind utilities so the last wins", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
    expect(cn("text-static-400", "text-signal")).toBe("text-signal");
  });
});
