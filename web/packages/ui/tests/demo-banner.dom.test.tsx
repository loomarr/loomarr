// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DemoBanner } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

const text = () => document.body.textContent ?? "";

describe("DemoBanner", () => {
  it("is a labelled region that says it's the demo and offers the next step as a link", async () => {
    await mount(
      <DemoBanner
        next={{ label: "Watch a channel live", link: <a href="/watch/ch-1">placeholder</a> }}
        onDismiss={vi.fn()}
        step={1}
        stepCount={3}
      />,
    );

    expect(document.body.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Demo tour");
    expect(text()).toContain("You're exploring the Loomarr demo.");
    expect(text()).toContain("Step 1 of 3");
    const link = document.body.querySelector('a[href="/watch/ch-1"]');
    expect(link?.textContent).toBe("Watch a channel live");
    expect(link?.getAttribute("role")).toBe("link");
  });

  it("says the tour is done once there is no next step", async () => {
    await mount(<DemoBanner onDismiss={vi.fn()} step={3} stepCount={3} />);

    expect(text()).toContain("That's the tour.");
    expect(text()).not.toContain("Step");
    expect(document.body.querySelector("a")).toBeNull();
  });

  it("hides with a named button", async () => {
    const onDismiss = vi.fn();
    await mount(<DemoBanner onDismiss={onDismiss} step={2} stepCount={3} />);

    const hide = document.body.querySelector<HTMLElement>('[aria-label="Hide the demo tour"]');
    expect(hide?.getAttribute("role")).toBe("button");
    await act(async () => hide?.click());

    expect(onDismiss).toHaveBeenCalledOnce();
  });

  // #2750: on a phone the banner wrapped into a message row, a next-step row and a row for the X,
  // about 100px of an 844px screen. Compact keeps one line: the Demo badge, the next step and the X.
  it("keeps to the badge, the next step and the X on a phone", async () => {
    await mount(
      <DemoBanner
        compact
        next={{ label: "Browse the Guide", link: <a href="/guide">placeholder</a> }}
        onDismiss={vi.fn()}
        step={2}
        stepCount={3}
      />,
    );

    expect(text()).toContain("Demo");
    expect(document.body.querySelector('a[href="/guide"]')?.textContent).toBe("Browse the Guide");
    expect(document.body.querySelector('[aria-label="Hide the demo tour"]')).not.toBeNull();
    expect(text()).not.toContain("You're exploring the Loomarr demo.");
    expect(text()).not.toContain("Step 2 of 3");
  });

  it("says the tour is done in a few words on a phone", async () => {
    await mount(<DemoBanner compact onDismiss={vi.fn()} step={3} stepCount={3} />);

    expect(text()).toContain("That's the tour.");
    expect(text()).not.toContain("explore freely");
  });
});
