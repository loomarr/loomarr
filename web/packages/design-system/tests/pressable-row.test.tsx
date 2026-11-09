// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { LoomarrProvider, PressableRow, Text } from "../index";

const markupOf = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("PressableRow", () => {
  it("is one button named by its content", () => {
    const markup = markupOf(
      <PressableRow>
        <Text textRole="body">Morning Cartoons</Text>
      </PressableRow>,
    );
    expect(markup).toContain('role="button"');
    expect(markup).toContain("Morning Cartoons");
  });

  it("takes the role the caller names", () => {
    expect(markupOf(<PressableRow accessibilityRole="option">Austin</PressableRow>)).toContain(
      'role="option"',
    );
  });

  it("draws the current row differently from the rest", () => {
    expect(markupOf(<PressableRow selected>A</PressableRow>)).not.toEqual(
      markupOf(<PressableRow>A</PressableRow>),
    );
  });

  it("announces selected for an option but not for a plain button", () => {
    expect(
      markupOf(
        <PressableRow accessibilityRole="option" selected>
          A
        </PressableRow>,
      ),
    ).toContain('aria-selected="true"');
    expect(markupOf(<PressableRow selected>A</PressableRow>)).not.toContain("aria-selected");
  });

  it("draws a hairline only when divided", () => {
    expect(markupOf(<PressableRow divided>A</PressableRow>)).not.toEqual(
      markupOf(<PressableRow>A</PressableRow>),
    );
  });

  it("runs onPress, and not when disabled", () => {
    const onPress = vi.fn();
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <LoomarrProvider theme="dark">
          <PressableRow onPress={onPress}>Open</PressableRow>
          <PressableRow disabled onPress={onPress}>
            Shut
          </PressableRow>
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
