// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider, Text } from "../index";

afterEach(cleanup);

const hostOf = (ui: React.ReactNode) => {
  render(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);
  return screen.getByTestId("host");
};

describe("Text display roles", () => {
  it("draws a role at its measured size, line height and weight", () => {
    const host = hostOf(
      <Text as="span" data-testid="host" textRole="microBadge">
        00:30
      </Text>,
    );

    expect(host.style.fontSize).toBe("9px");
    expect(host.style.lineHeight).toBe("13.5px");
    expect(host.style.fontWeight).toBe("400");
  });

  it("sets a mono role in the data family and a sans role in the body family", () => {
    const mono = hostOf(
      <Text as="span" data-testid="host" textRole="monogram">
        LM
      </Text>,
    );
    expect(mono.style.fontFamily).toContain("Mono");
    cleanup();
    const sans = hostOf(
      <Text as="span" data-testid="host" textRole="stationName">
        Saturday Cartoons
      </Text>,
    );
    expect(sans.style.fontFamily).not.toContain("Mono");
  });

  it("tracks a role out by its em letter spacing at its own size", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" textRole="pairCode">
        4F7-Q2K
      </Text>,
    );

    expect(host.style.letterSpacing).toBe("9.6px");
  });

  it("scales a fluid role with the window, its spacing in em and its height a multiple of the size", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" textRole="stationName">
        Saturday Cartoons
      </Text>,
    );

    expect(host.style.fontSize).toBe("clamp(18px, 2.8vw, 28px)");
    expect(host.style.letterSpacing).toBe("0.01em");
    expect(host.style.lineHeight).toBe("1.5");
  });

  it("takes the tag's size as 0.62 of the name's at every width", () => {
    const host = hostOf(
      <Text as="span" data-testid="host" textRole="stationTag">
        CH 7
      </Text>,
    );

    expect(host.style.fontSize).toBe("clamp(11.16px, 1.736vw, 17.36px)");
    expect(host.style.letterSpacing).toBe("0.16em");
    expect(host.style.lineHeight).toBe("1.5");
  });

  it("sets a line height of one with leading none, in place of the role's own", () => {
    const host = hostOf(
      <Text as="span" data-testid="host" leading="none" textRole="compact">
        ✕
      </Text>,
    );

    expect(host.style.fontSize).toBe("13px");
    expect(host.style.lineHeight).toBe("1");
  });

  it("lets an explicit tracking replace a role's letter spacing", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" textRole="pairCode" tracking={0.1}>
        4F7-Q2K
      </Text>,
    );

    expect(host.style.letterSpacing).toBe("3.2px");
  });
});
