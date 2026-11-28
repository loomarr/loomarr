// @vitest-environment jsdom
import type { GuideLayout } from "@loomarr/core/guide";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { declarationsOfElement } from "@loomarr/test-support/css";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GuideSurface } from "../guide";

type GuideNode = HTMLElement;
type QueryRoot = ParentNode;

const airing = (scheduleBlockId: string, startMs: number, isOnNow: boolean) => ({
  channelId: "harbor",
  clippedStartMs: startMs,
  clippedStopMs: startMs + 1_800_000,
  endsAfterWindow: false,
  isOnNow,
  progressRatio: isOnNow ? 0.5 : undefined,
  scheduleBlockId,
  source: {
    kind: "program" as const,
    scheduleBlockId,
    series: "Copper Valley Rangers",
    startMs,
    stopMs: startMs + 1_800_000,
    title: "Copper Valley Rangers",
  },
  startRatio: startMs / 3_600_000,
  startsBeforeWindow: false,
  widthRatio: 0.5,
});

const layout = {
  channels: [
    {
      airings: [airing("now", 0, true), airing("next", 1_800_000, false)],
      source: {
        airings: [],
        channelId: "harbor",
        name: "Harbor Lights",
        number: 7,
        pendingCount: 0,
        status: "live" as const,
      },
    },
  ],
  fromMs: 0,
  source: { channels: [], fromMs: 0, timezone: "UTC", toMs: 3_600_000 },
  timezone: "UTC",
  toMs: 3_600_000,
} satisfies GuideLayout;

// Tamagui draws a token colour as a hashed atomic class; read the declarations it carries.
const amberBorder = /(?<![-\w])border(?:-top)?-color:var\(--actionFocus\)/;
const amberText = /(?<![-\w])color:var\(--actionPrimary\)/;
// Harbor Lights' monogram ident is lettered in its hue's text stop, and that hue is the brand amber
// (#3011): identity, not focus, so the decorative tile is not counted.
const inIdent = (node: Element) => node.closest('[aria-hidden="true"]')?.textContent === "HL";
const isAmber = (node: Element) => {
  if (inIdent(node)) return false;
  const styles = declarationsOfElement(node);
  return amberBorder.test(styles) || amberText.test(styles);
};
const ambers = (host: QueryRoot) => Array.from(host.querySelectorAll("*")).filter(isAmber);

describe("TV guide focus (#1887)", () => {
  let host: HTMLElement;

  beforeEach(() => {
    // The dedicated ten-foot guide renders from a 1280px canvas.
    vi.stubGlobal("innerWidth", 1280);
    host = renderUi(
      <GuideSurface
        density="tv"
        filter="all"
        filters={[
          { label: "All", value: "all" },
          { label: "Recent", value: "recent" },
        ]}
        layout={layout}
        onSelectionChange={vi.fn()}
        selection={{ anchorMs: 900_000, channelId: "harbor", scheduleBlockId: "now" }}
      />,
    ).container;
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  // A programme's label is "<Channel>, <series>, <time range>"; the on-now one starts the window.
  const nowCell = () => {
    const pressable = host.querySelector(
      '[aria-label^="Harbor Lights, Copper Valley Rangers, 12:00"]',
    ) as GuideNode;
    return { block: pressable.firstElementChild as GuideNode, pressable };
  };
  const filter = (name: string) => host.querySelector(`[aria-label^="${name},"]`) as GuideNode;

  it("lights the selected programme and its channel number while the grid has focus", () => {
    expect(declarationsOfElement(nowCell().block)).toMatch(amberBorder);
    // The block's border and the row's number: both mark where the remote is.
    expect(ambers(host)).toHaveLength(2);
  });

  it("draws a now-airing block as a neutral tone step, not amber", () => {
    act(() => filter("Recent").focus());
    const { block } = nowCell();
    const styles = declarationsOfElement(block);
    expect(styles).toContain("background-color:var(--stateAiringSurface)");
    expect(styles).toMatch(/border(?:-top)?-color:var\(--borderAiring\)/);
  });

  it("drops the selection to rest when a filter takes focus, leaving the filter as the one amber", () => {
    act(() => filter("Recent").focus());
    const { block } = nowCell();

    expect(declarationsOfElement(block)).not.toMatch(amberBorder);
    expect(declarationsOfElement(block)).toMatch(/border(?:-top)?-width:1px/);
    // Only the focused filter's border and label remain amber; the grid has none left.
    const remaining = ambers(host);
    expect(remaining.length).toBeGreaterThan(0);
    expect(remaining.every((node) => node.textContent?.startsWith("Recent"))).toBe(true);

    act(() => nowCell().pressable.focus());
    expect(declarationsOfElement(nowCell().block)).toMatch(amberBorder);
    expect(ambers(host)).toHaveLength(2);
  });
});
