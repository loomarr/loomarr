// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { SearchButton } from "../index";

const doc = document;

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

type Clickable = { click(): void };

describe("SearchButton", () => {
  it("is a button named by its placeholder, with the shortcut hint and aria-keyshortcuts, that presses", async () => {
    let pressed = 0;
    await mount(
      <SearchButton hint="⌘K" keyShortcuts="Meta+K" onPress={() => (pressed += 1)} placeholder="Search…" />,
    );

    const button = doc.body.querySelector('[role="button"]') as unknown as
      | (Clickable & { getAttribute(name: string): string | null; textContent: string | null })
      | null;
    expect(button).not.toBeNull();
    expect(button?.textContent).toBe("Search…⌘K");
    expect(button?.getAttribute("aria-keyshortcuts")).toBe("Meta+K");
    // It draws a box and presses; it is not an input.
    expect(doc.body.querySelector("input")).toBeNull();
    await act(async () => button?.click());
    expect(pressed).toBe(1);
  });
});
