// @vitest-environment jsdom
import type { GuideLayout } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GuideSurface } from "../index";
import { declarationsOfElement } from "./support/atomic-css";

// The package type-checks without the DOM lib, so the test names only the node surface it reads.
type GuideNode = {
  className: string;
  firstElementChild: GuideNode | null;
  focus(): void;
  getAttribute(name: string): string | null;
  textContent: string | null;
};
type QueryRoot = {
  querySelector(selector: string): GuideNode | null;
  querySelectorAll(selector: string): ArrayLike<GuideNode>;
};
type Host = Parameters<typeof createRoot>[0] & QueryRoot & { remove(): void };
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tagName: string): Host };
  }
).document;

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
const isAmber = (node: Element) => {
  const styles = declarationsOfElement(node);
  return amberBorder.test(styles) || amberText.test(styles);
};
const ambers = (host: QueryRoot) => Array.from(host.querySelectorAll("*")).filter(isAmber);

describe("TV guide focus (#1887)", () => {
  let host: Host;
  let root: Root;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    // The dedicated ten-foot guide renders from a 1280px canvas.
    vi.stubGlobal("innerWidth", 1280);
    host = testDocument.createElement("div");
    testDocument.body.append(host);
    root = createRoot(host);
    act(() => {
      root.render(
        <LoomarrProvider>
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
          />
        </LoomarrProvider>,
      );
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
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
