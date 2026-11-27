// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LetterRail, LoomarrProvider } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Button = {
  click(): void;
  getAttribute(name: string): string | null;
  hasAttribute(name: string): boolean;
  textContent: string | null;
  firstElementChild: object | null;
};
const computed = (
  globalThis as unknown as {
    getComputedStyle(node: object): Record<"width" | "height" | "maxHeight" | "overflowY", string>;
  }
).getComputedStyle;
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Button | null;
  querySelectorAll(s: string): ArrayLike<Button>;
  remove(): void;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const mount = (node: React.ReactNode) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() => root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>));
  return {
    host,
    unmount: () => {
      act(() => root.unmount());
      host.remove();
    },
  };
};

const screen = vi.hoisted(() => ({ height: 900 }));
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return {
    ...actual,
    useWindowDimensions: () => ({ ...actual.useWindowDimensions(), height: screen.height }),
  };
});

describe("LetterRail", () => {
  it("marks the letter that is loading as busy", () => {
    const { host, unmount } = mount(<LetterRail available={["B", "M"]} busy="M" onSelect={() => {}} />);
    const busy = Array.from(host.querySelectorAll('[aria-busy="true"]')).map((b) => b.textContent);
    expect(busy).toEqual(["M"]);
    unmount();
  });

  // WCAG 2.5.8. 27 letters at 24 px are 648 px, more than a short window holds.
  it("keeps every letter 24 by 24 px in a short window, and scrolls the rail instead", () => {
    screen.height = 560;
    try {
      const { host, unmount } = mount(<LetterRail available={[]} onSelect={() => {}} />);
      const sizes = Array.from(host.querySelectorAll("button"), (button) => {
        const box = button.firstElementChild;
        if (!box) throw new Error("no letter");
        return `${computed(box).width} ${computed(box).height}`;
      });
      expect(new Set(sizes)).toEqual(new Set(["24px 24px"]));
      const rail = host.querySelector('[role="navigation"]');
      if (!rail) throw new Error("no rail");
      expect(Number.parseFloat(computed(rail).maxHeight)).toBeLessThanOrEqual(560);
      expect(computed(rail).overflowY).toBe("scroll");
      unmount();
    } finally {
      screen.height = 900;
    }
  });

  it("draws # then A to Z, disables the letters with nothing, and jumps to the one pressed", () => {
    const onSelect = vi.fn();
    const { host, unmount } = mount(
      <LetterRail available={["#", "B", "M"]} current="B" onSelect={onSelect} />,
    );
    expect(host.querySelector('[role="navigation"]')?.getAttribute("aria-label")).toBe("Jump to a letter");

    const buttons = Array.from(host.querySelectorAll("button"));
    expect(buttons.map((b) => b.textContent).join("")).toBe("#ABCDEFGHIJKLMNOPQRSTUVWXYZ");
    expect(buttons.filter((b) => !b.hasAttribute("disabled")).map((b) => b.textContent)).toEqual([
      "#",
      "B",
      "M",
    ]);
    expect(
      buttons.filter((b) => b.getAttribute("aria-current") === "true").map((b) => b.textContent),
    ).toEqual(["B"]);

    const pressed = (letter: string) => buttons.find((b) => b.textContent === letter) as Button;
    act(() => pressed("M").click());
    act(() => pressed("C").click());
    expect(onSelect.mock.calls).toEqual([["M"]]);
    unmount();
  });
});
