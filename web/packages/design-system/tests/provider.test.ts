import { describe, expect, it } from "vitest";

import { resolveLoomarrTheme } from "../index";

describe("Loomarr theme selection", () => {
  it("defaults a fresh client to dark regardless of the host appearance", () => {
    expect(resolveLoomarrTheme(undefined, "light")).toBe("dark");
    expect(resolveLoomarrTheme(undefined, "dark")).toBe("dark");
  });

  it("honors an explicit light preference", () => {
    expect(resolveLoomarrTheme("light", "dark")).toBe("light");
  });

  it("follows the host only when system mode was explicitly selected", () => {
    expect(resolveLoomarrTheme("system", "light")).toBe("light");
    expect(resolveLoomarrTheme("system", "dark")).toBe("dark");
    expect(resolveLoomarrTheme("system", null)).toBe("dark");
  });
});
