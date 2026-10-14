// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { LoomarrProvider, RadioGroup, RadioItem, Text } from "../index";

const VALUES = ["a", "b", "c"] as const;

const Harness = ({ initial }: { initial: string | undefined }) => {
  const [value, setValue] = useState(initial);
  return (
    <LoomarrProvider>
      <RadioGroup label="Letters" onValueChange={setValue} value={value} values={VALUES}>
        {VALUES.map((v) => (
          <RadioItem key={v} label={`Letter ${v}`} value={v}>
            {({ checked }) => <Text textRole="label">{checked ? `[${v}]` : v}</Text>}
          </RadioItem>
        ))}
      </RadioGroup>
    </LoomarrProvider>
  );
};

describe("RadioGroup", () => {
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  const radios = () => [...host.querySelectorAll<HTMLElement>('[role="radio"]')];
  const press = (el: HTMLElement, key: string) =>
    act(() => {
      el.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, key }));
    });

  it("is one tab stop: the selected item", () => {
    act(() => root.render(<Harness initial="b" />));
    expect(radios().map((r) => r.getAttribute("tabindex"))).toEqual(["-1", "0", "-1"]);
    expect(host.querySelector('[role="radiogroup"]')?.getAttribute("aria-label")).toBe("Letters");
  });

  it("falls back to the first item when nothing is selected", () => {
    act(() => root.render(<Harness initial={undefined} />));
    expect(radios().map((r) => r.getAttribute("tabindex"))).toEqual(["0", "-1", "-1"]);
  });

  it("arrow keys move focus and the selection, wrapping at the ends", () => {
    act(() => root.render(<Harness initial="a" />));
    press(radios()[0] as HTMLElement, "ArrowRight");
    expect(radios()[1]?.getAttribute("aria-checked")).toBe("true");
    expect(document.activeElement).toBe(radios()[1]);
    press(radios()[1] as HTMLElement, "ArrowDown");
    press(radios()[2] as HTMLElement, "ArrowRight");
    expect(radios()[0]?.getAttribute("aria-checked")).toBe("true");
    press(radios()[0] as HTMLElement, "ArrowLeft");
    expect(radios()[2]?.getAttribute("aria-checked")).toBe("true");
    expect(radios().map((r) => r.getAttribute("tabindex"))).toEqual(["-1", "-1", "0"]);
  });

  it("draws a focus ring only while an item has focus", () => {
    act(() => root.render(<Harness initial="a" />));
    // The ring is drawn in the $actionFocus token, which Tamagui names in its atomic class.
    const ring = () => host.querySelectorAll('[class*="_btc-actionFocu"]').length;
    const before = ring();
    act(() => radios()[0]?.focus());
    expect(ring()).toBeGreaterThan(before);
    act(() => radios()[0]?.blur());
    expect(ring()).toBe(before);
  });
});
