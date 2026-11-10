// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

// The window the guide is drawn in, in points; the grid reads it for the rail and the ruler.
const window = vi.hoisted(() => ({ width: 1280 }));

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: window.width }) };
});

const { GuideGrid } = await import("../index");

const day = Date.UTC(2026, 8, 28);
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const LONG_NAME = "Cozy Autumn Horror Night";

const source = {
  channels: [
    {
      airings: [
        {
          kind: "program",
          scheduleBlockId: "b1",
          series: "An invented anthology",
          startMs: at(19),
          stopMs: at(21),
          title: "The first night",
        },
      ],
      channelId: "ch1",
      name: LONG_NAME,
      number: 7,
      pendingCount: 3,
      status: "live",
    },
  ],
  fromMs: at(19),
  timezone: "UTC",
  toMs: at(22),
} as unknown as Parameters<typeof layoutGuide>[0];

const render = (nowMs: number) => {
  window.width = 1280;
  return renderToStaticMarkup(
    <LoomarrProvider>
      <GuideGrid layout={layoutGuide(source, nowMs)} nowMs={nowMs} />
    </LoomarrProvider>,
  );
};

describe("the guide grid's text case (#3410)", () => {
  it("sets the series name and the row chip in sentence case, not capitals", () => {
    const html = render(at(19, 30));
    expect(html).toContain("An invented anthology");
    expect(html).toContain("Still downloading");
    expect(html).not.toContain("STILL DOWNLOADING");
    // Neither the series name's nor the chip's text carries an uppercase transform.
    expect(html).not.toMatch(/text-transform:\s*uppercase/);
  });
});

describe("the guide grid's channel rail (#3410)", () => {
  it("lets a 24-character name wrap to two lines instead of cutting it at 1280 points", () => {
    expect(LONG_NAME).toHaveLength(24);
    const html = render(at(19, 30));
    expect(html).toContain(LONG_NAME);
    // A Text limited to one line is nowrap (`_ws-`); one limited to N lines is a line clamp (`_wlc-`).
    const name = html.match(new RegExp(`<span class="[^"]*">${LONG_NAME}</span>`))?.[0] ?? "";
    expect(name).toContain("_wlc-");
    expect(name).not.toContain("_ws-");
  });
});

describe("the guide grid's ruler (#3410)", () => {
  it("hides the hour's label when the now label would sit on it, keeping both at half past", () => {
    const near = render(at(19, 55));
    expect(near).toContain("7:55 PM");
    expect(near).not.toContain("8:00 PM");
    // The other hours keep their labels.
    expect(near).toContain("9:00 PM");

    const mid = render(at(19, 30));
    expect(mid).toContain("7:30 PM");
    expect(mid).toContain("8:00 PM");
  });

  it("hides the label of the hour just passed, too", () => {
    const html = render(at(20, 4));
    expect(html).toContain("8:04 PM");
    expect(html).not.toContain("8:00 PM");
  });
});
