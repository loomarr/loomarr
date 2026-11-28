// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider, Text, type TextRole } from "../index";

afterEach(cleanup);

const hostOf = (textRole: TextRole, density: "pointer" | "touch" | "tv") => {
  render(
    <LoomarrProvider theme="dark">
      <Text as="span" data-testid="host" density={density} textRole={textRole}>
        7
      </Text>
    </LoomarrProvider>,
  );
  const { style } = screen.getByTestId("host");
  const drawn = [style.fontSize, style.lineHeight, style.fontWeight, style.fontFamily.includes("Mono")];
  cleanup();
  return drawn;
};

describe("Text lower third roles", () => {
  it.each([
    ["thirdNumeral", "pointer", "64px", "64px", "500", true],
    ["thirdNumeral", "touch", "44px", "44px", "500", true],
    ["thirdNumeral", "tv", "52px", "52px", "500", true],
    ["thirdTitle", "pointer", "34px", "38px", "600", false],
    ["thirdTitle", "touch", "22px", "25px", "600", false],
    ["thirdTitle", "tv", "30px", "34px", "600", false],
    ["thirdNumeralWindow", "pointer", "56px", "56px", "500", true],
    ["thirdTitleWindow", "pointer", "28px", "31px", "600", false],
    ["thirdNumeralSide", "touch", "40px", "40px", "500", true],
  ] as const)("draws %s at %s", (role, density, size, lineHeight, weight, mono) => {
    expect(hostOf(role, density)).toEqual([size, lineHeight, weight, mono]);
  });
});
