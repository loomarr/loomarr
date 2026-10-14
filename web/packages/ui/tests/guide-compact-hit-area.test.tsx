// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import type { PressableProps } from "react-native";
import { describe, expect, it, vi } from "vitest";

// The tap area is the Pressable's own box, the only one react-native-web hit-tests (it ignores
// hitSlop), so the test records the box the guide gives each cell's Pressable.
const cells = vi.hoisted(
  () => [] as { hitSlop?: unknown; label: string; left: number; width: number; zIndex?: number }[],
);

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return {
    ...actual,
    // A 390 pt phone: the timeline is that less the page gutters and the number column.
    useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: 390 }),
    Pressable: (props: PressableProps) => {
      const style = props.style as { left?: number; width?: number; zIndex?: number } | undefined;
      if (typeof style?.width === "number" && props.accessibilityLabel?.startsWith("1 Channel 1,")) {
        cells.push({
          hitSlop: props.hitSlop,
          label: props.accessibilityLabel,
          left: style.left ?? 0,
          width: style.width,
          zIndex: style.zIndex,
        });
      }
      return <actual.Pressable {...props} />;
    },
  };
});

const { GuideCompact } = await import("../index");

const MINUTE = 60_000;
const FROM = Date.UTC(2026, 8, 28, 20, 0);
const TIMELINE_PX = 390 - 2 * 16 - 40;

const airing = (id: string, from: number, to: number) => ({
  kind: "program" as const,
  scheduleBlockId: id,
  startMs: FROM + from * MINUTE,
  stopMs: FROM + to * MINUTE,
  series: id,
  title: id,
});

const layout = layoutGuide(
  {
    fromMs: FROM,
    toMs: FROM + 120 * MINUTE,
    timezone: "UTC",
    channels: [
      {
        channelId: "ch-1",
        name: "Channel 1",
        number: 1,
        pendingCount: 0,
        status: "live" as const,
        airings: [
          airing("opening", -30, 3),
          airing("middle", 3, 60),
          airing("brief", 60, 66),
          airing("rest", 66, 118),
          airing("closing", 118, 150),
        ],
      },
    ],
  },
  FROM + 90 * MINUTE,
);

const render = (selectedBlockId?: string) => {
  cells.length = 0;
  renderToStaticMarkup(
    <LoomarrProvider>
      <GuideCompact
        filter="all"
        layout={layout}
        myChannels={{ favouriteIds: [], recentIds: [] }}
        nowMs={FROM + 90 * MINUTE}
        onFilterChange={vi.fn()}
        onSelect={vi.fn()}
        onWatch={vi.fn()}
        selection={
          selectedBlockId
            ? { anchorMs: FROM, channelId: "ch-1", scheduleBlockId: selectedBlockId }
            : undefined
        }
      />
    </LoomarrProvider>,
  );
  return new Map(cells.map((cell) => [cell.label.split(", ")[1]?.split(" · ")[0] ?? "", cell]));
};

// Where each airing is drawn on the timeline, in points.
const PX_PER_MINUTE = TIMELINE_PX / 120;
const drawn = { brief: [60, 66], closing: [118, 120], middle: [3, 60], opening: [0, 3] } as const;

describe("GuideCompact tap areas (Q-N6)", () => {
  it("reaches 44 pt for every airing in the box the web hit-tests, not a hitSlop", () => {
    const byName = render();
    expect([...byName.keys()]).toEqual(["opening", "middle", "brief", "rest", "closing"]);
    for (const cell of byName.values()) {
      expect(cell.width).toBeGreaterThanOrEqual(44 - 1e-6);
      expect(cell.hitSlop).toBeUndefined();
    }
  });

  it("centres a short cell's tap area on it, widens a clipped one toward the window, and leaves a long one alone", () => {
    const byName = render();
    const [from, to] = drawn.brief;
    const brief = byName.get("brief");
    // 6 minutes of a 2 hour window is about 16 pt wide on a phone; its 44 pt straddles it evenly.
    expect((to - from) * PX_PER_MINUTE).toBeLessThan(44);
    expect((brief?.left ?? 0) + (brief?.width ?? 0) / 2).toBeCloseTo(((from + to) / 2) * PX_PER_MINUTE);
    expect(byName.get("opening")?.left).toBeCloseTo(drawn.opening[0] * PX_PER_MINUTE);
    expect((byName.get("closing")?.left ?? 0) + (byName.get("closing")?.width ?? 0)).toBeCloseTo(
      drawn.closing[1] * PX_PER_MINUTE,
    );
    expect(byName.get("middle")?.left).toBeCloseTo(drawn.middle[0] * PX_PER_MINUTE);
    expect(byName.get("middle")?.width).toBeCloseTo((drawn.middle[1] - drawn.middle[0]) * PX_PER_MINUTE);
  });

  // 9 PM sits 5 minutes before this window's end, 14 pt from the edge on a phone: its label would
  // wrap to "9 / PM" against it.
  it("leaves an hour unlabelled when its label has no room before the ruler's edge", () => {
    const from = Date.UTC(2026, 8, 28, 19, 15);
    const to = from + 110 * MINUTE;
    const edge = layoutGuide(
      {
        fromMs: from,
        toMs: to,
        timezone: "UTC",
        channels: [
          {
            channelId: "ch-1",
            name: "Channel 1",
            number: 1,
            pendingCount: 0,
            status: "live" as const,
            airings: [airing("film", -75, 35)],
          },
        ],
      },
      from,
    );
    const html = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideCompact
          filter="all"
          layout={edge}
          myChannels={{ favouriteIds: [], recentIds: [] }}
          nowMs={from - 30 * MINUTE}
          onFilterChange={vi.fn()}
          onSelect={vi.fn()}
          onWatch={vi.fn()}
        />
      </LoomarrProvider>,
    );
    // Intl may space the meridiem with a narrow no-break space.
    expect(html).toMatch(/>8\sPM</);
    expect(html).not.toMatch(/>9\sPM</);
  });

  it("lets the selected cell win an overlap, then the narrower of two cells", () => {
    const byName = render("middle");
    const z = (name: string) => byName.get(name)?.zIndex ?? -1;
    expect(z("middle")).toBeGreaterThan(z("brief"));
    expect(z("brief")).toBeGreaterThan(z("rest"));
    expect(z("rest")).toBe(0);
    // The 2-minute closing cell is narrower than the 6-minute brief one, so it wins their overlap.
    expect(z("closing")).toBeGreaterThan(z("brief"));
  });
});
