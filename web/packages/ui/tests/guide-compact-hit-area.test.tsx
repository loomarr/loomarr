// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import type { PressableProps } from "react-native";
import { describe, expect, it, vi } from "vitest";

// react-native-web ignores hitSlop, so the test records what the guide hands each cell's Pressable.
const cells = vi.hoisted(
  () => [] as { hitSlop?: { left: number; right: number }; label: string; width: number; zIndex?: number }[],
);

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return {
    ...actual,
    // A 390 pt phone: the timeline is that less the page gutters and the number column.
    useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: 390 }),
    Pressable: (props: PressableProps) => {
      const style = props.style as { width?: string; zIndex?: number } | undefined;
      if (typeof style?.width === "string" && props.accessibilityLabel?.startsWith("1 Channel 1,")) {
        cells.push({
          hitSlop: props.hitSlop as { left: number; right: number } | undefined,
          label: props.accessibilityLabel,
          width: Number.parseFloat(style.width) / 100,
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

const hitWidth = (cell: { hitSlop?: { left: number; right: number }; width: number }) =>
  cell.width * TIMELINE_PX + (cell.hitSlop?.left ?? 0) + (cell.hitSlop?.right ?? 0);

describe("GuideCompact tap areas (Q-N6)", () => {
  it("reaches 44 pt for every airing, the clipped ones at the window's edges included", () => {
    const byName = render();
    expect([...byName.keys()]).toEqual(["opening", "middle", "brief", "rest", "closing"]);
    for (const cell of byName.values()) expect(hitWidth(cell)).toBeGreaterThanOrEqual(44 - 1e-6);
    // 6 minutes of a 2 hour window is about 16 pt wide on a phone.
    expect(byName.get("brief")?.width).toBeLessThan(44 / TIMELINE_PX);
  });

  it("widens a clipped cell toward the window, and leaves a long cell alone", () => {
    const byName = render();
    expect(byName.get("opening")?.hitSlop?.left).toBe(0);
    expect(byName.get("closing")?.hitSlop?.right).toBe(0);
    expect(byName.get("brief")?.hitSlop?.left).toBeCloseTo(byName.get("brief")?.hitSlop?.right ?? -1);
    expect(byName.get("middle")?.hitSlop).toBeUndefined();
  });

  it("lets the selected cell win an overlap, then a widened one over a wide neighbour", () => {
    const byName = render("middle");
    expect(byName.get("middle")?.zIndex).toBe(2);
    expect(byName.get("brief")?.zIndex).toBe(1);
    expect(byName.get("rest")?.zIndex).toBe(0);
  });
});
