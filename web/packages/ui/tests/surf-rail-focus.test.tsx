// @vitest-environment jsdom
import { LoomarrProvider } from "@loomarr/design-system";
import { surfGroups } from "@loomarr/fixtures";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SurfRail } from "../index";

// The package type-checks without the DOM lib, so the test names only the node surface it reads.
type RailNode = {
  className: string;
  firstElementChild: RailNode | null;
  focus(): void;
  style?: { borderTopColor: string };
  textContent: string | null;
};
type QueryRoot = {
  querySelector(selector: string): RailNode | null;
  querySelectorAll(selector: string): ArrayLike<RailNode>;
};
type Host = Parameters<typeof createRoot>[0] & QueryRoot & { remove(): void };
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tagName: string): Host };
  }
).document;

// The selected card draws focus through Tamagui's atomic `$actionFocus` class; an Action draws it
// as react-native-web's inline border colour. Either is an amber on screen.
const amberCard = (node: RailNode) => /\b_btc-actionFocus\b/.test(node.className);
const amberAction = (node: RailNode) => node.style?.borderTopColor === "rgb(255, 176, 32)";
const ambers = (host: QueryRoot) =>
  Array.from(host.querySelectorAll("*")).filter((node) => amberCard(node) || amberAction(node));

describe("TV surf rail focus (#1887)", () => {
  const channel = surfGroups.find((group) => group.kind === "recent")?.channels[0];
  let host: Host;
  let root: Root;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    // The dedicated ten-foot rail renders from a 1280px canvas.
    vi.stubGlobal("innerWidth", 1280);
    host = testDocument.createElement("div");
    testDocument.body.append(host);
    root = createRoot(host);
    act(() => {
      root.render(
        <LoomarrProvider>
          <SurfRail
            autoTune={{ choicesMs: [1_200, 5_000], onChange: vi.fn(), valueMs: 5_000 }}
            clientVersion="prototype"
            density="tv"
            groups={surfGroups}
            onDisconnect={vi.fn()}
            onFocusSelection={vi.fn()}
            onTune={vi.fn()}
            selection={{ channelId: channel?.id ?? "", group: "recent" }}
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

  const row = () =>
    host.querySelector(`[aria-label^="recent, channel ${channel?.channelNumber},"]`) as RailNode;
  const card = () => row().firstElementChild as RailNode;
  const button = (label: string) =>
    Array.from(host.querySelectorAll('[role="button"]')).find((node) =>
      node.textContent?.startsWith(label),
    ) as RailNode;

  it("draws the selected card as the one amber while the list has focus", () => {
    expect(amberCard(card())).toBe(true);
    expect(ambers(host)).toEqual([card()]);
  });

  it("drops the selected card to rest when a rail action takes focus, keeping it expanded", () => {
    const expanded = card().textContent;
    expect(expanded).not.toBe(`${channel?.channelNumber.padStart(2, "0")}${channel?.channelName}`);
    const wait = button("Channel-number wait");
    act(() => wait.focus());

    expect(amberCard(card())).toBe(false);
    // Tamagui shortens a long token in its class name (`_btc-borderDeco…`).
    expect(card().className).toMatch(/\b_btc-borderDeco/);
    // Still the selection OK returns to: the programme line and progress stay.
    expect(card().textContent).toBe(expanded);
    expect(ambers(host)).toEqual([wait]);

    act(() => button("Disconnect device").focus());
    expect(ambers(host)).toEqual([button("Disconnect device")]);
  });

  it("lights the card again when focus returns to the list", () => {
    act(() => button("Channel-number wait").focus());
    act(() => row().focus());

    expect(amberCard(card())).toBe(true);
    expect(ambers(host)).toEqual([card()]);
  });
});
