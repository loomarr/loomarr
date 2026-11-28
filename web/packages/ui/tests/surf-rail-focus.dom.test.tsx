// @vitest-environment jsdom
import { surfGroups } from "@loomarr/fixtures";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { declarationsOfElement } from "@loomarr/test-support/css";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SurfRail } from "../index";

type RailNode = HTMLElement;
type QueryRoot = ParentNode;

// The selected card draws focus through a Tamagui atomic class whose rule names `actionFocus`; an
// Action draws it as react-native-web's inline border colour. Either is an amber on screen.
const amberCard = (node: RailNode) =>
  /(?<![-\w])border(?:-top)?-color:var\(--actionFocus\)/.test(declarationsOfElement(node));
const amberAction = (node: RailNode) => node.style?.borderTopColor === "rgb(255, 176, 32)";
const ambers = (host: QueryRoot) =>
  Array.from(host.querySelectorAll<HTMLElement>("*")).filter((node) => amberCard(node) || amberAction(node));

describe("TV surf rail focus (#1887)", () => {
  const channel = surfGroups.find((group) => group.kind === "recent")?.channels[0];
  let host: HTMLElement;
  let view: ReturnType<typeof renderUi>;

  beforeEach(() => {
    // The dedicated ten-foot rail renders from a 1280px canvas.
    vi.stubGlobal("innerWidth", 1280);
    view = renderUi(
      <SurfRail
        autoTune={{ choicesMs: [1_200, 5_000], onChange: vi.fn(), valueMs: 5_000 }}
        clientVersion="prototype"
        density="tv"
        groups={surfGroups}
        onDisconnect={vi.fn()}
        onFocusSelection={vi.fn()}
        onTune={vi.fn()}
        selection={{ channelId: channel?.id ?? "", group: "recent" }}
      />,
    );
    host = view.container;
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  // The row is named by its content (WCAG 2.5.3), so it is found by what it shows: the padded
  // channel's ident initials, its padded number, then its name; the recent group lists it first.
  const row = () =>
    Array.from(host.querySelectorAll('[role="button"]')).find((node) =>
      node.textContent?.includes(`${channel?.channelNumber.padStart(2, "0")}${channel?.channelName}`),
    ) as RailNode;
  const card = () => row().firstElementChild as RailNode;
  // The channel number is the card's data-mono text: its padded digits.
  const number = () =>
    Array.from(host.querySelectorAll(".font_data")).find(
      (node) => node.textContent === channel?.channelNumber.padStart(2, "0"),
    ) as RailNode;
  const button = (label: string) =>
    Array.from(host.querySelectorAll('[role="button"]')).find((node) =>
      node.textContent?.startsWith(label),
    ) as RailNode;

  it("draws the selected card as the one amber while the list has focus", () => {
    expect(amberCard(card())).toBe(true);
    expect(declarationsOfElement(card())).toMatch(/(?<![-\w])border(?:-top)?-width:3px/);
    expect(declarationsOfElement(number())).toMatch(/(?<![-\w])color:var\(--actionPrimary\)/);
    expect(ambers(host)).toEqual([card()]);
  });

  it("drops the selected card to rest when a rail action takes focus, keeping it expanded", () => {
    const expanded = card().textContent;
    expect(expanded).not.toBe(`${channel?.channelNumber.padStart(2, "0")}${channel?.channelName}`);
    const wait = button("Channel-number wait");
    act(() => wait.focus());

    expect(amberCard(card())).toBe(false);
    // At rest: a hairline, not the 3px focus border, and a muted number. Tamagui shortens a long
    // token in its rule (`border-color:var(--borderDecorative)`).
    expect(declarationsOfElement(card())).toMatch(
      /(?<![-\w])border(?:-top)?-color:var\(--borderDecorative\)/,
    );
    expect(declarationsOfElement(card())).toMatch(/(?<![-\w])border(?:-top)?-width:1px/);
    expect(declarationsOfElement(number())).toMatch(/(?<![-\w])color:var\(--contentMuted\)/);
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

  // The focus-target wiring every TV surface shares: a journey's registry gets each card's handle
  // and hears where focus lands before the surface's own handler runs, and a surface rendered
  // without a registry still reports its selection.
  const renderRail = (
    onFocusSelection: () => void,
    focusRegistry?: { focused: () => void; register: () => void; request: () => void },
  ) =>
    view.rerender(
      <SurfRail
        autoTune={{ choicesMs: [1_200, 5_000], onChange: vi.fn(), valueMs: 5_000 }}
        clientVersion="prototype"
        density="tv"
        focusRegistry={focusRegistry}
        groups={surfGroups}
        onDisconnect={vi.fn()}
        onFocusSelection={onFocusSelection}
        onTune={vi.fn()}
        selection={{ channelId: channel?.id ?? "", group: "recent" }}
      />,
    );

  it("registers each card with the focus registry and reports focus to it before the rail", () => {
    const calls: string[] = [];
    const registry = { focused: vi.fn(() => calls.push("registry")), register: vi.fn(), request: vi.fn() };
    renderRail(() => calls.push("rail"), registry);

    act(() => row().focus());

    const selection = { channelId: channel?.id, group: "recent" };
    expect(registry.register).toHaveBeenCalledWith(selection, expect.anything());
    expect(registry.focused).toHaveBeenCalledWith(selection);
    expect(calls).toEqual(["registry", "rail"]);
  });

  it("reports the focused selection without a registry", () => {
    const onFocusSelection = vi.fn();
    renderRail(onFocusSelection);

    act(() => row().focus());

    expect(onFocusSelection).toHaveBeenCalledWith({ channelId: channel?.id, group: "recent" });
  });
});
