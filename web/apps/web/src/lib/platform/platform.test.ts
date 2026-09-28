import { describe, expect, it } from "vitest";
import { commandShortcutAria, commandShortcutLabel, isApplePlatform } from "./platform";

describe("platform", () => {
  it("names ⌘K on Apple platforms and Ctrl K elsewhere (#1659)", () => {
    expect(commandShortcutLabel({ platform: "MacIntel" })).toBe("⌘K");
    expect(commandShortcutLabel({ platform: "iPhone" })).toBe("⌘K");
    expect(commandShortcutLabel({ platform: "Win32" })).toBe("Ctrl K");
    expect(commandShortcutLabel({ platform: "Linux x86_64" })).toBe("Ctrl K");
    expect(commandShortcutAria({ platform: "MacIntel" })).toBe("Meta+K");
    expect(commandShortcutAria({ platform: "Win32" })).toBe("Control+K");
  });

  // Chromium is freezing `navigator.platform`; its client hint is the better source when present.
  it("prefers the client hint over navigator.platform", () => {
    expect(isApplePlatform({ platform: "", userAgentData: { platform: "macOS" } })).toBe(true);
    expect(isApplePlatform({ platform: "MacIntel", userAgentData: { platform: "Windows" } })).toBe(false);
  });

  it("falls back to Ctrl when the platform is unknown", () => {
    expect(isApplePlatform({ platform: "" })).toBe(false);
  });
});
