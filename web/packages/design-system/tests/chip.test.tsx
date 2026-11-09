// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { Chip, LoomarrProvider } from "../index";

const markupOf = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("Chip", () => {
  it("announces a toggle's state for both on and off", () => {
    expect(markupOf(<Chip selected>2H</Chip>)).toContain('aria-pressed="true"');
    expect(markupOf(<Chip selected={false}>2H</Chip>)).toContain('aria-pressed="false"');
  });

  it("leaves aria-pressed off a chip that only runs an action", () => {
    const markup = markupOf(<Chip>back to today</Chip>);
    expect(markup).not.toContain("aria-pressed");
    expect(markup).toContain("back to today");
  });

  it("draws an on chip differently from an off one", () => {
    const on = markupOf(<Chip selected>2H</Chip>);
    const off = markupOf(<Chip selected={false}>2H</Chip>);
    expect(on).not.toEqual(off);
  });

  it("names a chip with its accessibilityLabel", () => {
    expect(markupOf(<Chip accessibilityLabel="Show 2 hours">2H</Chip>)).toContain(
      'aria-label="Show 2 hours"',
    );
  });

  it("runs onPress and not when disabled", () => {
    const onPress = vi.fn();
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <Chip onPress={onPress}>Newest first</Chip>
          <Chip disabled onPress={onPress}>
            Year
          </Chip>
        </LoomarrProvider>,
      );
    });
    const [first, second] = [...container.querySelectorAll<HTMLElement>('[role="button"]')];
    act(() => {
      first?.click();
      second?.click();
    });
    expect(onPress).toHaveBeenCalledTimes(1);
    act(() => root.unmount());
    container.remove();
  });
});
