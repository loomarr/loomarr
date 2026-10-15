import { describe, expect, it } from "vitest";

import { nativeFontFace } from "../index";

describe("native font faces", () => {
  it("names each embedded weight by PostScript name on iOS and by file name on Android", () => {
    expect(nativeFontFace("body", "ios", "600")).toBe("Geist-SemiBold");
    expect(nativeFontFace("body", "android", "600")).toBe("Geist_600SemiBold");
    expect(nativeFontFace("data", "ios", 500)).toBe("GeistMono-Medium");
    expect(nativeFontFace("data", "android")).toBe("GeistMono_400Regular");
  });

  it("draws a weight it doesn't embed the way a browser would from the same static files", () => {
    // The type scale's off-grid weights: 550 tries heavier first, as does 650.
    expect(nativeFontFace("body", "ios", "550")).toBe("Geist-SemiBold");
    expect(nativeFontFace("body", "ios", "650")).toBe("Geist-Bold");
    // Geist Mono stops at 600, so heavier mono text draws SemiBold rather than the system font.
    expect(nativeFontFace("data", "ios", "650")).toBe("GeistMono-SemiBold");
    expect(nativeFontFace("data", "ios", "bold")).toBe("GeistMono-SemiBold");
    // Lighter than anything embedded falls back up to Regular; 450 tries up to 500 first.
    expect(nativeFontFace("body", "ios", "300")).toBe("Geist-Regular");
    expect(nativeFontFace("body", "ios", "450")).toBe("Geist-Medium");
    expect(nativeFontFace("body", "ios", "normal")).toBe("Geist-Regular");
  });
});
