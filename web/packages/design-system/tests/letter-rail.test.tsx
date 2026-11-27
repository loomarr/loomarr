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
};
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

describe("LetterRail", () => {
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
