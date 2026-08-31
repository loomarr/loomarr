import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@tamagui/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tamagui/core")>();
  return { ...actual, isWeb: false };
});

const { LoomarrProvider, Text } = await import("../index");

describe("universal typography weights", () => {
  it("renders only system-font weights through the native host", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Text density="tv" textRole="time">
          9:30 PM
        </Text>
        <Text density="tv" textRole="title">
          Programme title
        </Text>
      </LoomarrProvider>,
    );

    expect(markup).toContain("font-weight:600");
    expect(markup).toContain("font-weight:700");
    expect(markup).not.toContain("font-weight:550");
    expect(markup).not.toContain("font-weight:650");
  });
});
